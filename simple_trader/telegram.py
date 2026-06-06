# Simple-Trader\simple_trader\telegram.py
# -*- coding: utf-8 -*-
"""
Telegram notifier wrapper to send messages to a bot chat.

This module defines a `TelegramNotifier` class that is used by the signal generator
to publish formatted trade signals and analysis messages via Telegram Bot API.

Key points:
- Respects `backtest_mode` from the project config: if true, messages are not actually sent.
- Supports formatting signals based on `db.Signal` dataclass or sqlite row objects.
- Uses a simple retry/backoff for transient network/telegram issues.
- Provides `send_message`, `send_signal`, `send_analysis_summary`, and `send_batch` helpers.
- Escapes MarkdownV2 special characters in message content to avoid formatting errors.

Example usage:

    from simple_trader.telegram import TelegramNotifier
    from simple_trader.config import CONFIG

    notifier = TelegramNotifier(config=CONFIG)
    notifier.send_message("Hello world!")  # sends to default chat configured

    # If you have a `Signal` dataclass instance:
    notifier.send_signal(my_signal)
"""

from __future__ import annotations

import json
import logging
import math
import time
from dataclasses import asdict
from typing import Any, Dict, List, Optional, Union

import requests

from simple_trader.config import CONFIG, Config
from simple_trader.db import Database, get_default_db

logger = logging.getLogger("simple_trader.telegram")
logger.addHandler(logging.NullHandler())


def _escape_markdown_v2(text: str) -> str:
    """
    Escape MarkdownV2 special characters so the Telegram bot doesn't try to parse them.
    MarkdownV2 special characters:
        _ * [ ] ( ) ~ ` > # + - = | { } . !
    Reference: https://core.telegram.org/bots/api#markdownv2-style
    """
    if not isinstance(text, str) or not text:
        return text or ""
    # Order of replacement matters in some cases; keep it explicit
    # Avoid escaping dots in URLs: escape all MarkdownV2 special characters except '.' when the text looks like a URL
    text = text.replace("\\", "\\\\")
    # Simple heuristic: do not escape dots in plain text (avoid \"5\.3%\" artifacts)
    # and avoid escaping dots in URLs for link targets.
    if isinstance(text, str) and text.startswith("http"):
        # escape everything except '.' and ':' and '/'
        chars_to_escape = "_*[]()~`>#+-=|{}!"
    else:
        chars_to_escape = "_*[]()~`>#+-=|{}!"
    for ch in chars_to_escape:
        text = text.replace(ch, f"\\{ch}")
    return text


def _fmt_price(
    symbol: str, price: Optional[float], decimals: Optional[int] = None
) -> str:
    """
    Format price with a reasonable number of decimals:
    - For > 1: 2 decimals
    - For < 1: 6 decimals (common in forex/low-valued coins)
    - Accept optional decimals override.
    """
    if price is None:
        return "N/A"
    if decimals is None:
        if price >= 1.0:
            decimals = 2
        else:
            decimals = 6
    fmt = f"{{:,.{decimals}f}}"
    try:
        return fmt.format(float(price))
    except Exception:
        return str(price)


class TelegramNotifier:
    """
    TelegramNotifier wraps Telegram Bot API `sendMessage` and provides helper functions
    to create well-structured messages.

    The object uses `config.telegram_bot_token` and `config.telegram_chat_id` by default,
    but they can also be specified at construction time.
    """

    TELEGRAM_API_BASE = "https://api.telegram.org"

    def __init__(
        self,
        token: Optional[str] = None,
        chat_id: Optional[Union[int, str]] = None,
        config: Optional[Config] = None,
        db: Optional[Database] = None,
        dry_run: Optional[bool] = None,
    ):
        self.config = config or CONFIG
        self.token = token or self.config.telegram_bot_token
        self.chat_id = chat_id or self.config.telegram_chat_id
        self.db = db or get_default_db(
            self.config.database_path, tenant_id=self.config.tenant_id
        )
        self.session = requests.Session()
        self.session.headers.update({"User-Agent": "SimpleTrader-Telegram/1.0"})
        # If nil or not set, notifies will be no-op to avoid accidental messages
        self.dry_run = self.config.backtest_mode if dry_run is None else dry_run

    def _validate(self) -> bool:
        if self.dry_run:
            logger.info(
                "TelegramNotifier dry-run/backtest_mode enabled: messages will not be sent."
            )
            # Return True to indicate local formatting is OK, but we won't actually send.
            return False
        if not self.token:
            logger.warning(
                "Telegram bot token is not configured. Skipping notifications."
            )
            return False
        if not self.chat_id:
            logger.warning("Telegram chat_id not configured. Skipping notifications.")
            return False
        return True

    def send_message(
        self,
        text: str,
        parse_mode: str = "MarkdownV2",
        disable_web_page_preview: bool = True,
        disable_notification: bool = False,
        reply_markup: Optional[Dict] = None,
        max_retries: int = 3,
        retry_backoff: float = 0.5,
    ) -> Optional[Dict[str, Any]]:
        """
        Send a plain text message via Telegram Bot API. Returns the parsed JSON response on success
        or None if the message was not sent (dry_run or misconfigured).
        """
        if not self._validate():
            # Format and display the message locally for dev or logging
            logger.info("Telegram notification (mock): %s", text)
            return None

        url = f"{self.TELEGRAM_API_BASE}/bot{self.token}/sendMessage"
        payload = {
            "chat_id": str(self.chat_id),
            "text": text,
            "parse_mode": parse_mode,
            "disable_web_page_preview": disable_web_page_preview,
            "disable_notification": disable_notification,
        }
        if reply_markup:
            payload["reply_markup"] = json.dumps(reply_markup)

        # send with simple retry loop for transient status codes
        attempt = 0
        last_exception = None
        while attempt < max_retries:
            attempt += 1
            try:
                resp = self.session.post(url, json=payload, timeout=20)
                if resp.status_code == 200:
                    try:
                        return resp.json()
                    except Exception:
                        return {"ok": True, "result": None}
                if resp.status_code in (429, 500, 502, 503, 504):
                    # often transient; backoff and retry
                    logger.warning(
                        "Telegram returned %s, retry attempt %s: %s",
                        resp.status_code,
                        attempt,
                        resp.text,
                    )
                    # respect Retry-After header where present (for 429)
                    retry_after = resp.headers.get("Retry-After")
                    if retry_after:
                        try:
                            wait = float(retry_after)
                        except Exception:
                            wait = retry_backoff * attempt
                    else:
                        wait = retry_backoff * attempt
                    time.sleep(wait)
                    continue
                # Non-retriable error -> log and return
                logger.error(
                    "Telegram API error: status=%s body=%s", resp.status_code, resp.text
                )
                return {"ok": False, "status_code": resp.status_code, "body": resp.text}
            except Exception as e:
                last_exception = e
                logger.exception(
                    "Exception while sending Telegram message (attempt %s): %s",
                    attempt,
                    e,
                )
                time.sleep(retry_backoff * attempt)

        logger.error(
            "Failed to send Telegram message after %s attempts; last exception: %s",
            max_retries,
            last_exception,
        )
        return None

    def _signal_to_message(self, signal: Any) -> str:
        """
        Given a `Signal` dataclass or a DB row (Mapping-like), format a textual summary
        suitable for sending via Telegram (MarkdownV2 escaped).
        The message contains:
          - asset symbol and direction
          - entry, SL, TP, leverage, rr
          - risk/position size (if available)
          - created_at & expiry
          - optionally the top LLM analysis summary & news link
        """
        # Accept either dataclass or sqlite Row (mapping)
        try:
            # If we have a dataclass (like Signal), `asdict` will work; if it's a sqlite Row, treat as mapping.
            if hasattr(signal, "__dataclass_fields__"):
                d = asdict(signal)
            else:
                # sqlite row or dict-like
                d = dict(signal)
        except Exception:
            # fallback: try mapping directly
            if isinstance(signal, dict):
                d = signal
            else:
                d = {}

        symbol = d.get("symbol") or d.get("ticker") or "UNKNOWN"
        side = (d.get("side") or "unknown").upper()
        entry_price = d.get("entry_price")
        stop_loss = d.get("stop_loss")
        take_profit = d.get("take_profit")
        leverage = d.get("leverage") or d.get("recommended_leverage") or None
        rr = d.get("rr")
        timeframe = d.get("timeframe_hours") or self.config.timeframe_hours
        created_at = d.get("created_at") or d.get("created") or None
        if isinstance(created_at, (int, float)):
            created_str = time.strftime(
                "%Y-%m-%d %H:%M:%S UTC", time.gmtime(int(created_at))
            )
        else:
            created_str = str(created_at or "")
        expires_at = d.get("expires_at") or d.get("expiry") or None
        try:
            # Attempt to convert expire to readable format if iso-like
            if isinstance(expires_at, str) and expires_at:
                expires_at_str = expires_at
            elif isinstance(expires_at, (int, float)):
                expires_at_str = time.strftime(
                    "%Y-%m-%d %H:%M:%S UTC", time.gmtime(int(expires_at))
                )
            else:
                expires_at_str = ""
        except Exception:
            expires_at_str = str(expires_at or "")

        # See if we can fetch the latest LLM analysis for news_id (if present)
        news_id = d.get("news_id")
        analysis_summary = None
        analysis_provider = None
        if news_id and hasattr(self.db, "get_latest_analysis"):
            try:
                an = self.db.get_latest_analysis(int(news_id))
                if an and an["analysis_json"]:
                    aj = (
                        json.loads(an["analysis_json"])
                        if isinstance(an["analysis_json"], str)
                        else an["analysis_json"]
                    )
                    analysis_summary = (
                        aj.get("summary") or aj.get("analysis") or aj.get("explanation")
                    )
                    analysis_provider = an.get("provider")
            except Exception:
                logger.debug("Unable to fetch latest analysis for news_id=%s", news_id)

        # Compute risk size if missing: derive from configured risk per trade and account balance
        position_size = d.get("position_size")
        risk_amount = d.get("risk_amount")
        # Try to compute if entry_price and SL are known and risk_amount not set
        if risk_amount is None and entry_price and stop_loss:
            diff = abs(entry_price - stop_loss)
            if diff > 0.0:
                # position_value_usd = (account_balance_usd * risk_per_trade_pct) / (diff/entry_price)
                account = self.config.account_balance_usd
                risk_pct = self.config.risk_per_trade_pct
                risk_amount = max(0.0, account * risk_pct)
                # estimate nominal position in base asset
                # caution: in leveraged positions, this is naive; still, signal message provides raw USD values
                position_size_est = (risk_amount / diff) if diff > 0 else None
                position_size = position_size or position_size_est

        # Format message with MarkdownV2 escaping
        # Use math / emoji to present clearly
        def esc(x: Any) -> str:
            return _escape_markdown_v2(str(x)) if x is not None else "N/A"

        lines: List[str] = []
        lines.append(f"*📣 New Trade Signal*: *{esc(symbol)}*")
        lines.append(f"_Side_: *{esc(side)}*  — _Timeframe_: *{esc(timeframe)}h*")
        if entry_price:
            lines.append(f"_Entry_: *{esc(_fmt_price(symbol, entry_price))}*")
        if stop_loss:
            lines.append(f"_Stop Loss_: *{esc(_fmt_price(symbol, stop_loss))}*")
        if take_profit:
            lines.append(f"_Take Profit_: *{esc(_fmt_price(symbol, take_profit))}*")
        if rr:
            lines.append(f"_R:R_: *{esc(rr)}x*")
        if leverage:
            lines.append(f"_Leverage_: *{esc(str(leverage))}x*")
        if position_size:
            lines.append(f"_Position Size_: *{esc(position_size)}*")
        if risk_amount:
            lines.append(f"_Risk Amount_: *{esc(_fmt_price(symbol, risk_amount))}* USD")
        if analysis_provider or analysis_summary:
            provider_txt = (
                f"(via {esc(analysis_provider)})" if analysis_provider else ""
            )
            summary_text = esc(analysis_summary) if analysis_summary else ""
            if summary_text:
                lines.append(f"_Analysis_ {provider_txt}: *{summary_text}*")
        # if news is available, attempt to show url
        news_url = None
        try:
            if news_id and hasattr(self.db, "get_news"):
                # DB doesn't have get by id method; we'll query using `get_news` and filter (lightweight)
                # But to avoid heavy queries we call execute_custom.
                rows = self.db.execute_custom(
                    "SELECT url, title FROM news WHERE id = ? LIMIT 1", (int(news_id),)
                )
                if rows:
                    news_row = rows[0]
                    news_url = news_row["url"]
                    news_title = news_row["title"]
                    lines.append(f"_Source_: {esc(news_title)}")
                    # include URL as raw (no escaping for link)
                    # Use MarkdownV2 formatted link:
                    try:
                        url_esc = _escape_markdown_v2(news_url)
                        lines.append(f"[View article]({news_url})")
                    except Exception:
                        # fallback plain text
                        lines.append(esc(news_url))
        except Exception:
            logger.debug(
                "Error while attaching news url to message for news_id=%s", news_id
            )

        # created/expiry
        if created_str:
            lines.append(f"_Created_: `{esc(created_str)}`")
        if expires_at_str:
            lines.append(f"_Expires_: `{esc(expires_at_str)}`")
        lines.append("")  # newline
        lines.append("_Generated by Simple-Trader_")

        # Combine lines and ensure we minimize message size
        message_text = "\n".join(lines)
        # Telegram has a 4096 character limit; truncate gracefully if needed
        if len(message_text) > 4000:
            message_text = message_text[:3990] + "\n...[truncated]"

        return message_text

    def send_signal(
        self,
        signal: Any,
        parse_mode: str = "MarkdownV2",
        extra_buttons: Optional[List[Dict]] = None,
    ) -> Optional[Dict[str, Any]]:
        """
        Format and send a signal. Accepts dataclass `Signal` or DB row/dict.
        `extra_buttons` is an optional list of InlineKeyboardButton rows, e.g.:
        [
            [{"text": "More details", "url": "http://..."}, {"text": "Backtest", "callback_data": "bt_123"}]
        ]
        """
        message = self._signal_to_message(signal)
        reply_markup = None
        if extra_buttons:
            reply_markup = {"inline_keyboard": extra_buttons}
        return self.send_message(
            message, parse_mode=parse_mode, reply_markup=reply_markup
        )

    def send_analysis_summary(
        self, analysis: Dict[str, Any], parse_mode: str = "MarkdownV2"
    ) -> Optional[Dict[str, Any]]:
        """
        Send a short analysis summary (e.g., LLMSummary/analysis row).
        `analysis` may be a dict like { provider, summary, confidence, asset, direction } or DB row form.
        """
        if hasattr(analysis, "__dataclass_fields__"):
            ad = asdict(analysis)
        elif isinstance(analysis, dict):
            ad = analysis
        else:
            # attempt to handle sqlite row
            try:
                ad = dict(analysis)
            except Exception:
                ad = {"summary": str(analysis)}

        provider = ad.get("provider") or ad.get("source") or "analysis"
        summary_text = ad.get("summary") or ad.get("analysis") or ""
        asset = ad.get("asset") or ""
        direction = ad.get("direction") or ""
        confidence = ad.get("confidence")
        conf_str = f"{float(confidence):.2f}" if confidence is not None else "N/A"

        lines = []
        lines.append(f"*🔬 Analysis ({_escape_markdown_v2(provider)})*")
        if asset:
            lines.append(f"_Asset_: *{_escape_markdown_v2(asset)}*")
        if direction:
            lines.append(f"_Direction_: *{_escape_markdown_v2(direction)}*")
        lines.append(f"_Confidence_: *{_escape_markdown_v2(conf_str)}*")
        if summary_text:
            lines.append("")
            lines.append(f"{_escape_markdown_v2(summary_text)}")
        message = "\n".join(lines)
        return self.send_message(message, parse_mode=parse_mode)

    def send_batch(
        self, signals: List[Any], parse_mode: str = "MarkdownV2"
    ) -> List[Optional[Dict[str, Any]]]:
        """
        Send a batch of signals. Respects `dry_run` and returns a list of results (or None for skipped items).
        """
        results = []
        for s in signals:
            try:
                res = self.send_signal(s, parse_mode=parse_mode)
                results.append(res)
            except Exception:
                logger.exception(
                    "Failed to send signal via Telegram for signal=%s",
                    getattr(s, "symbol", s),
                )
                results.append(None)
        return results
