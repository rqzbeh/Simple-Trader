# Simple-Trader/simple_trader/llm_pool.py
# -*- coding: utf-8 -*-
"""
LLM Pool for Simple-Trader
==========================

This module provides a lightweight, production-friendly way to distribute news
analysis requests across multiple LLM providers, honoring per-provider
rate limits and offering a fallback strategy when a provider fails.

Key features:
- Provider adapters with a generic HTTP-based adapter and a mock adapter for testing.
- Rate limiting per provider (simple sliding window approach).
- Weighted distribution of items across providers (expanded-list round-robin).
- Automatic fallback: if a provider fails to analyze an item, try another provider.
- Optional logging of LLM usage into the database.

Design notes:
- The generic HTTP client attempts to call a provider's endpoint by sending JSON:
    {"prompt": "<prompt>", "max_tokens": 512}
  Most cloud providers differ in the exact API; the user should adapt the endpoint
  and payload for their provider if necessary.
- `MockLLMClient` returns deterministic, low-cost JSON that can be used in tests
  and in the absence of real provider credentials.
- The normalized analysis structure returned by `LLMClient.analyze()`:
    {
        "provider": "groq",
        "news_id": 123,
        "direction": "long" | "short" | "neutral",
        "impact_score": -1.0 .. 1.0,
        "confidence": 0.0 .. 1.0,
        "summary": "Short summary...",
        "recommended_leverage": int | None,
        "extra": {... provider-specific raw response ...}
    }
"""

from __future__ import annotations

import json
import logging
import math
import random
import threading
import time
from collections import deque
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Optional, Tuple

import requests

from simple_trader.config import Config, LLMProviderConfig, CONFIG
from simple_trader.db import AnalysisItem, Database, NewsItem, get_default_db, now_ts

logger = logging.getLogger("simple_trader.llm_pool")
logger.addHandler(logging.NullHandler())


@dataclass
class LLMSummary:
    provider: str
    news_id: Optional[int]
    direction: str  # 'long' | 'short' | 'neutral'
    impact_score: float  # normalized -1..1
    confidence: float  # 0..1
    summary: str
    asset: Optional[str] = None
    recommended_leverage: Optional[int] = None
    raw: Optional[Dict] = None


class RateLimiter:
    """
    Simple sliding-window rate limiter for "requests per minute".

    Implementation notes:
    - Uses a deque of timestamps, where we keep request timestamps for the last 60 seconds.
    - `acquire()` blocks until the request can proceed.
    """

    def __init__(self, requests_per_minute: int):
        self.rpm = max(1, int(requests_per_minute))
        self.timestamps = deque()
        self.lock = threading.Lock()
        self.window_seconds = 60.0

    def acquire(self) -> None:
        """Block until we can make another request in the rate limit window."""
        while True:
            with self.lock:
                now = time.time()
                # Drop timestamps older than `window_seconds`
                while self.timestamps and (now - self.timestamps[0]) > self.window_seconds:
                    self.timestamps.popleft()
                if len(self.timestamps) < self.rpm:
                    # we can proceed
                    self.timestamps.append(now)
                    return
                # calculate when the oldest timestamp will expire
                oldest = self.timestamps[0]
                wait = self.window_seconds - (now - oldest)
                # clamp wait
                if wait <= 0:
                    # will loop and try again
                    continue
            # sleep outside the lock
            time.sleep(max(wait, 0.01))


class LLMClient:
    """
    Abstract base class for LLM provider adapters.
    Subclasses implement `analyze()` which returns a normalized `LLMSummary`.
    """

    def __init__(self, provider_cfg: LLMProviderConfig, config: Optional[Config] = None, db: Optional[Database] = None):
        self.provider_cfg = provider_cfg
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path)
        self.rate_limiter = RateLimiter(self.provider_cfg.rate_limit_per_minute)
        self.session = requests.Session()
        # Allow provider-specific auth header name and optional API-key prefix (e.g., 'Bearer ').
        if self.provider_cfg.api_key:
            header_name = getattr(self.provider_cfg, "auth_header", None) or "Authorization"
            key_prefix = getattr(self.provider_cfg, "api_key_prefix", None)
            # Default to 'Bearer ' when the prefix is unspecified; allow explicit empty string to use raw key.
            if key_prefix is None:
                key_prefix = "Bearer "
            if key_prefix:
                header_value = f"{key_prefix}{self.provider_cfg.api_key}"
            else:
                header_value = str(self.provider_cfg.api_key)
            self.session.headers.update({header_name: header_value})
        # Basic UA header for provider calls
        self.session.headers.update({"User-Agent": "SimpleTrader-LLM/1.0"})
        # Maximum number of retries for transient errors
        self.max_retries = 2

    def name(self) -> str:
        return self.provider_cfg.name

    def construct_prompt(self, news_item: NewsItem) -> str:
        """
        Build a prompt that asks the LLM to analyze an individual news item and answer
        in a strict JSON structure. We use JSON output to keep parsing deterministic.

        The prompt requests an output like:
        { "direction": "long|short|neutral", "confidence": 0.0..1.0, "impact_score": -1..1, "summary": "...", "recommended_leverage": int|null }
        """
        text = (news_item.title or "") + "\n\n" + (news_item.content or "")
        # Keep the prompt compact but explicit
        prompt_template = (
            "You are a financial analyst specializing in short-term market moves. "
            "Given the news article below, analyze the likely short-term impact (2 hour timeframe) "
            "on the most relevant tradeable asset. Reply ONLY with valid JSON in the following format:\n\n"
            "{\n"
            '  "asset": "BTC|ETH|EURUSD|... or null",\n'
            '  "direction": "long" | "short" | "neutral",\n'
            '  "confidence": 0.0..1.0,  # probability/confidence in the recommendation\n'
            '  "impact_score": -1.0..1.0,  # negative for downward impact, positive for upward. Magnitude indicates strength.\n'
            '  "summary": "a brief 1-2 sentence explanation",\n'
            '  "recommended_leverage": integer or null\n'
            "}\n\n"
            "News Article:\n"
            f"{text}\n\n"
            "Constraints: consider that this is for a 2-hour timeframe and that trades should not be held "
            "for more than 24 hours. Favor conservative leverage, and state null for leverage if unsure.\n\n"
            "Be concise and return only JSON."
        )
        return prompt_template

    def analyze(self, news_item: NewsItem) -> LLMSummary:
        """
        Public analyze method. Subclasses implement `_call_provider_api`.
        The base method handles rate limiting and simple retry logic.
        """
        # Rate-limit for provider (blocks if necessary)
        self.rate_limiter.acquire()

        last_exc = None
        for attempt in range(1, self.max_retries + 2):
            try:
                with time_llm_request(self.name()):
                    raw_resp = self._call_provider_api(news_item)
                # parse and normalize response
                analysis = self._normalize_response(raw_resp, news_item)
                # log usage (best-effort; we estimate token counts)
                try:
                    request_ts = now_ts()
                    request_size = len(self.construct_prompt(news_item)) // 4
                    response_tokens = len(json.dumps(raw_resp)) // 4
                    est_cost = 0.0
                    self.db.log_llm_usage(self.name(), request_ts, request_size, response_tokens, est_cost)
                except Exception:
                    # Don't fail if logging fails
                    pass
                return analysis
            except Exception as e:
                last_exc = e
                logger.exception("LLM provider %s failed on attempt %s: %s", self.name(), attempt, e)
                # Exponential backoff between retries
                time.sleep(min(2 ** attempt * 0.2, 3.0))
                continue

        # If we get here, all retries failed
        raise RuntimeError(f"LLM provider {self.name()} failed after retries: {last_exc}")

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        """
        Provider-specific API call. The default implementation uses an
        OpenAI-compatible Chat Completions (messages) format: {"model": "...", "messages": [...]}.
        Subclasses can override this method for provider-specific payload shapes if necessary.
        """
        endpoint = self.provider_cfg.endpoint or ""  # must be provided by provider config
        if not endpoint:
            raise RuntimeError(f"No endpoint configured for provider '{self.name()}'")
        prompt = self.construct_prompt(news_item)
        model = getattr(self.provider_cfg, "model", None)
        if not model:
            provider_name = (self.name() or "").lower()
            if provider_name == "groq":
                model = "llama3-8b-8192"
            else:
                model = "gpt-4o-mini"
        # Basic system instruction to help ensure consistent JSON output from providers.
        system_message = {
            "role": "system",
            "content": (
                "You are a pragmatic, concise financial analyst specializing in short-term (2-hour) market moves. "
                "Given a news article, reply ONLY with valid JSON in the format: "
                '{"asset": "SYMBOL", "direction": "long|short|neutral", "confidence": 0.0..1.0, '
                '"impact_score": -1.0..1.0, "summary": "short explanation", "recommended_leverage": integer|null}. '
                "Be concise and include no extra text."
            ),
        }
        messages = [system_message, {"role": "user", "content": prompt}]
        payload = {"model": model, "messages": messages, "temperature": 0.0, "max_tokens": 512}
        logger.debug("LLM %s calling endpoint %s (model=%s, len prompt=%s)", self.name(), endpoint, model, len(prompt))
        resp = self.session.post(endpoint, json=payload, timeout=20)
        if resp.status_code >= 400:
            raise RuntimeError(f"LLM provider {self.name()} returned HTTP {resp.status_code}: {resp.text}")
        try:
            return resp.json()
        except Exception:
            # Some providers return plain text; wrap it in a simple structure
            text = resp.text
            return {"raw_text": text}

    def _normalize_response(self, raw_resp: Dict[str, Any], news_item: NewsItem) -> LLMSummary:
        """
        Convert a raw provider response to our normalized LLMSummary dataclass.

        The strategy:
        - If the response includes an explicit JSON string, parse it.
        - Otherwise, try to extract a JSON blob from text.
        - If parsing fails, fall back to a sentiment heuristic (not ideal).
        """
        parsed = None
        # 1) If the provider gave us a top-level JSON structure that contains the desired fields, pick it
        for candidate in ("json", "data", "result", "choices", "outputs", "responses", "candidates", "predictions", "output", "completion", "raw_json"):
            if candidate in raw_resp:
                val = raw_resp[candidate]
                # Accept either a dict or a list; prefer the first element when a list is returned
                if isinstance(val, dict):
                    parsed = val
                    break
                if isinstance(val, list) and val:
                    first = val[0]
                    if isinstance(first, dict):
                        # Try to extract normalized message shapes commonly found across vendors:
                        # - { 'message': { 'content': {...} } }
                        # - { 'content': {...} }
                        # - { 'text': '...'}
                        if "message" in first and isinstance(first["message"], dict):
                            msg_content = first["message"].get("content")
                            parsed = msg_content if isinstance(msg_content, dict) else first["message"]
                        elif "content" in first and isinstance(first["content"], dict):
                            parsed = first["content"]
                        elif "text" in first:
                            # When the candidate provides plain text, wrap for downstream JSON extraction
                            parsed = {"raw_text": first.get("text")}
                        else:
                            parsed = first
                    else:
                        parsed = {"raw_text": str(first)}
                    break

        if parsed is None:
            # If 'raw_text' is provided by the generic _call_provider_api fallback, attempt to parse JSON from text
            if "raw_text" in raw_resp and isinstance(raw_resp["raw_text"], str):
                parsed = self._extract_json_from_text(raw_resp["raw_text"])
            elif "choices" in raw_resp and isinstance(raw_resp["choices"], list) and raw_resp["choices"]:
                # Many LLM APIs return a 'choices' list with 'text' or 'message' field
                choice = raw_resp["choices"][0]
                text = choice.get("text") or choice.get("message") or choice.get("content") or ""
                if isinstance(text, str):
                    parsed = self._extract_json_from_text(text)
            else:
                # As last resort, the raw response might be a flat dict with fields we can map
                if isinstance(raw_resp, dict):
                    parsed = raw_resp

        if not parsed:
            # Fallback to simple neutral analysis
            logger.debug("LLM %s returned no JSON, falling back to neutral summary", self.name())
            return LLMSummary(
                provider=self.name(),
                news_id=news_item.published_at,
                direction="neutral",
                impact_score=0.0,
                confidence=0.0,
                summary="No parseable analysis",
                recommended_leverage=None,
                raw=raw_resp,
            )

        # Now interpret normalized keys
        # Accept multiple naming conventions to be robust
        direction = parsed.get("direction") or parsed.get("recommendation") or parsed.get("side") or "neutral"
        direction = direction.lower() if isinstance(direction, str) else "neutral"
        if direction not in ("long", "short", "neutral"):
            # attempt to normalize other values:
            if str(direction).lower().startswith("buy"):
                direction = "long"
            elif str(direction).lower().startswith("sell"):
                direction = "short"
            else:
                direction = "neutral"

        # Confidence
        confidence = parsed.get("confidence")
        if confidence is None:
            # try 'probability' or 'certainty'
            confidence = parsed.get("probability") or parsed.get("certainty") or parsed.get("score")
        confidence = self._as_float(confidence, default=0.0)
        # Clip confidence to 0..1
        confidence = max(0.0, min(1.0, float(confidence)))

        # Impact score; many prompts use -1..1 or -5..5
        impact_score = parsed.get("impact_score") or parsed.get("impact") or parsed.get("magnitude")
        impact_score = self._as_float(impact_score, default=0.0)
        # Normalize to -1..1 if magnitude > 1
        if abs(impact_score) > 1.0:
            impact_score = max(-1.0, min(1.0, impact_score / max(1.0, abs(impact_score))))
        # summary
        summary = parsed.get("summary") or parsed.get("explanation") or parsed.get("reason") or ""
        if not summary and isinstance(parsed, str):
            # If parsed is a string, use it
            summary = parsed

        # leverage
        leverage = parsed.get("recommended_leverage") or parsed.get("leverage")
        try:
            if leverage is not None:
                leverage = int(leverage)
        except Exception:
            leverage = None

        # Build normalized LLMSummary
        asset = None
        # Try to find an 'asset' in the parsed response
        if isinstance(parsed, dict):
            asset = parsed.get("asset") or parsed.get("asset_symbol") or parsed.get("symbol") or asset
        # Fallback to the DB-provided hint if available
        if not asset and getattr(news_item, "asset", None):
            asset = news_item.asset
        asset = asset.strip().upper() if isinstance(asset, str) and asset.strip() else None

        summary_obj = LLMSummary(
            provider=self.name(),
            news_id=news_item.published_at,
            asset=asset,
            direction=direction,
            impact_score=float(impact_score),
            confidence=float(confidence),
            summary=str(summary) if summary is not None else "",
            recommended_leverage=leverage,
            raw=parsed if isinstance(parsed, dict) else {"raw": parsed},
        )
        return summary_obj

    def _extract_json_from_text(self, text: str) -> Optional[Dict[str, Any]]:
        """
        Attempt to find and parse a JSON object in a text blob.
        Returns dict or None.
        """
        # First, attempt to parse the entire text
        try:
            return json.loads(text)
        except Exception:
            pass
        # Find first { ... } block
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end >= 0 and end > start:
            snippet = text[start:end + 1]
            try:
                return json.loads(snippet)
            except Exception:
                pass
        return None

    @staticmethod
    def _as_float(value: Any, default: float = 0.0) -> float:
        if value is None:
            return default
        try:
            return float(value)
        except Exception:
            try:
                return float(str(value).strip().replace("%", "")) / 100.0 if "%" in str(value) else default
            except Exception:
                return default


class GenericHTTPLLMClient(LLMClient):
    """
    Generic HTTP-based LLM adapter useful for providers that accept a JSON `{"prompt": ...}`
    POST payload and return JSON. This will most likely require adapter tuning for real providers.
    """

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        endpoint = self.provider_cfg.endpoint
        if not endpoint:
            raise RuntimeError(f"Endpoint missing for provider {self.name()}")
        prompt = self.construct_prompt(news_item)
        model = getattr(self.provider_cfg, "model", None) or "gpt-4o-mini"
        system_message = {
            "role": "system",
            "content": (
                "You are a concise financial analyst for short-term (2H) signal generation. "
                "Given the news article content, reply ONLY with well-formed JSON containing "
                '{"asset","direction","confidence","impact_score","summary","recommended_leverage"} and no additional text.'
            ),
        }
        messages = [system_message, {"role": "user", "content": prompt}]
        payload = {"model": model, "messages": messages, "temperature": 0.0, "max_tokens": 512}
        # Some providers (e.g., Google, Groq) may require a slightly different shape; override if necessary.
        resp = self.session.post(endpoint, json=payload, timeout=25)
        if resp.status_code >= 400:
            raise RuntimeError(f"HTTP {resp.status_code} from {self.name()}: {resp.text}")
        try:
            return resp.json()
        except Exception:
            return {"raw_text": resp.text}


class GroqLLMClient(GenericHTTPLLMClient):
    """
    Groq-specific LLM adapter. Uses an OpenAI-like chat payload under the hood.
    Allows the endpoint to be the provider base and will append a sensible chat path
    (e.g., "/chat/completions") if necessary.
    """
    def __init__(self, provider_cfg: LLMProviderConfig, config: Optional[Config] = None, db: Optional[Database] = None):
        super().__init__(provider_cfg, config=config, db=db)

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        endpoint = self.provider_cfg.endpoint or "https://api.groq.ai/v1"
        # Append a default chat path if only a v1 base was given
        if endpoint.rstrip("/").endswith("/v1"):
            endpoint = endpoint.rstrip("/") + "/chat/completions"
        prompt = self.construct_prompt(news_item)
        model = getattr(self.provider_cfg, "model", None) or "gpt-4o-mini"
        system_message = {
            "role": "system",
            "content": (
                "You are a pragmatic, concise financial analyst specializing in short-term (2-hour) market moves. "
                "Reply ONLY with valid JSON in the format requested by the app."
            ),
        }
        messages = [system_message, {"role": "user", "content": prompt}]
        payload = {"model": model, "messages": messages, "temperature": 0.0, "max_tokens": 512}
        resp = self.session.post(endpoint, json=payload, timeout=25)
        if resp.status_code >= 400:
            raise RuntimeError(f"Groq provider HTTP {resp.status_code}: {resp.text}")
        try:
            return resp.json()
        except Exception:
            return {"raw_text": resp.text}


class CloudflareLLMClient(GenericHTTPLLMClient):
    """
    Cloudflare Workers/Models adapter.
    The endpoint should be provided explicitly (e.g., an account-level path). This adapter
    simply uses the configured endpoint + model and the configured auth header.
    """
    def __init__(self, provider_cfg: LLMProviderConfig, config: Optional[Config] = None, db: Optional[Database] = None):
        super().__init__(provider_cfg, config=config, db=db)

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        endpoint = self.provider_cfg.endpoint
        if not endpoint:
            raise RuntimeError("Endpoint missing for provider cloudflare")
        prompt = self.construct_prompt(news_item)
        model = getattr(self.provider_cfg, "model", None) or "gpt-4o-mini"
        system_message = {"role": "system", "content": "You are a concise financial analyst for short-term (2H) signal generation. Reply with JSON only."}
        messages = [system_message, {"role": "user", "content": prompt}]
        payload = {"model": model, "messages": messages, "temperature": 0.0, "max_tokens": 512}
        resp = self.session.post(endpoint, json=payload, timeout=25)
        if resp.status_code >= 400:
            raise RuntimeError(f"Cloudflare provider HTTP {resp.status_code}: {resp.text}")
        try:
            return resp.json()
        except Exception:
            return {"raw_text": resp.text}


class GoogleLLMClient(GenericHTTPLLMClient):
    """
    Google Gemini / Google AI adapter.
    If configured endpoint is a models base path (e.g., /v1beta2/models), this client appends
    a model-specific path like '.../{model}:predict'. The generic OpenAI-style payload is used by default.
    """
    def __init__(self, provider_cfg: LLMProviderConfig, config: Optional[Config] = None, db: Optional[Database] = None):
        super().__init__(provider_cfg, config=config, db=db)

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        base_endpoint = self.provider_cfg.endpoint or ""
        model = getattr(self.provider_cfg, "model", None) or "gpt-4o-mini"
        # If endpoint looks like a models base, call the model's predict/generate path
        if base_endpoint.rstrip("/").endswith("/models"):
            endpoint = f"{base_endpoint.rstrip('/')}/{model}:predict"
        else:
            endpoint = base_endpoint or ""
        if not endpoint:
            raise RuntimeError("No endpoint configured for Google provider")
        prompt = self.construct_prompt(news_item)
        system_message = {"role": "system", "content": "You are a concise financial analyst for short-term (2H) signal generation. Reply with JSON only."}
        messages = [system_message, {"role": "user", "content": prompt}]
        payload = {"model": model, "messages": messages, "temperature": 0.0, "max_tokens": 512}
        resp = self.session.post(endpoint, json=payload, timeout=25)
        if resp.status_code >= 400:
            raise RuntimeError(f"Google provider HTTP {resp.status_code}: {resp.text}")
        try:
            return resp.json()
        except Exception:
            return {"raw_text": resp.text}


class MockLLMClient(LLMClient):
    """
    Deterministic mock provider for development and tests: returns synthetic analysis
    based on simple heuristics from the article title and text.
    This mock will always return `confidence` in range (0.4..0.9) and `impact_score` within (-0.8..0.8).
    """

    def __init__(self, provider_cfg: LLMProviderConfig, config: Optional[Config] = None, db: Optional[Database] = None):
        # Ensure mock is always enabled; rate limit is irrelevant, but still created
        super().__init__(provider_cfg, config=config, db=db)

    def _call_provider_api(self, news_item: NewsItem) -> Dict[str, Any]:
        # Use a simple deterministic seed for reproducibility: hash of title + published_at
        seed_input = (news_item.title or "") + "|" + (str(news_item.published_at) or "")
        h = sum(ord(c) for c in seed_input) % 10000
        r = random.Random(h)
        # Heuristics: if title contains "hike" or "rate" -> likely negative for risk assets, etc.
        t = (news_item.title or "").lower()
        direction = "neutral"
        impact = 0.0
        if any(k in t for k in ["hike", "rate", "fed", "inflation", "recession", "sanction", "ban", "hack", "down"]):
            direction = "short"
            impact = -0.2 - r.random() * 0.5
        if any(k in t for k in ["upgrade", "partnership", "bull", "record high", "surge", "gain", "beat"]):
            direction = "long"
            impact = 0.2 + r.random() * 0.5
        if "bitcoin" in t or "btc" in t:
            # small chance to be strong impact
            impact += 0.2 * (r.random() - 0.5)
        confidence = 0.4 + r.random() * 0.5
        summary = (news_item.title or "")[:200]
        asset_guess = news_item.asset if getattr(news_item, "asset", None) else None
        result = {
            "json": {
                "direction": direction,
                "confidence": round(confidence, 3),
                "impact_score": round(max(-1.0, min(1.0, impact)), 3),
                "summary": summary,
                "asset": asset_guess,
                "recommended_leverage": None,
            }
        }
        # Simulate API latency artificially
        time.sleep(0.05 + (r.random() * 0.2))
        return result


class LLMPool:
    """
    Orchestrate multi-provider analysis of news items, honoring rate limits and provider weights.

    Typical usage:
        pool = LLMPool(config=cfg, db=db)
        results = pool.analyze_batch(news_items)
        # results: list of LLMSummary objects (one per news item) - the returned list order matches the input order
    """

    def __init__(self, config: Optional[Config] = None, db: Optional[Database] = None):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path)
        self.providers_cfg = [p for p in self.config.llm_providers if p.enabled]
        # Build client instances
        self.clients = self._init_clients_from_cfg(self.providers_cfg)
        # Make sure we have at least one provider (mock) - ideally the config already ensures this
        if not self.clients:
            # create a mock provider
            mock_cfg = LLMProviderConfig(name="mock", api_key=None, endpoint=None, rate_limit_per_minute=1000, enabled=True)
            self.clients = [MockLLMClient(mock_cfg, config=self.config, db=self.db)]
        # build expanded provider list for weighted distribution
        self.expand_providers = self._build_weighted_provider_list(self.clients)
        # a pool for concurrency; limit by `llm_global_concurrency`
        max_workers = max(1, min(32, self.config.llm_global_concurrency))
        self.executor = ThreadPoolExecutor(max_workers=max_workers)

    def shutdown(self, wait: bool = True) -> None:
        try:
            if getattr(self, "executor", None) is not None:
                self.executor.shutdown(wait=wait)
        except Exception:
            logger.exception("Failed to shut down LLMPool executor")

    def __del__(self):
        try:
            self.shutdown(wait=False)
        except Exception:
            pass

    def _init_clients_from_cfg(self, cfgs: Iterable[LLMProviderConfig]) -> List[LLMClient]:
        result: List[LLMClient] = []
        for c in cfgs:
            # Instantiate specific provider client based on name
            name_lower = (c.name or "").lower()
            if name_lower == "mock":
                client = MockLLMClient(c, config=self.config, db=self.db)
            elif name_lower == "groq":
                client = GroqLLMClient(c, config=self.config, db=self.db)
            elif name_lower in ("cloudflare", "cf"):
                client = CloudflareLLMClient(c, config=self.config, db=self.db)
            elif name_lower in ("google", "google_ai", "googleai"):
                client = GoogleLLMClient(c, config=self.config, db=self.db)
            else:
                # For most providers, we can use the GenericHTTP adapter by default
                client = GenericHTTPLLMClient(c, config=self.config, db=self.db)
            result.append(client)
        return result

    @staticmethod
    def _build_weighted_provider_list(clients: List[LLMClient]) -> List[LLMClient]:
        expanded: List[LLMClient] = []
        for c in clients:
            w = max(1, getattr(c.provider_cfg, "weight", 1))
            for _ in range(w):
                expanded.append(c)
        return expanded

    def adapt_weights(
        self,
        lookback_seconds: int = 60,
        low_usage_threshold: float = 0.4,
        high_usage_threshold: float = 0.85,
        max_weight: int = 5,
    ) -> None:
        """
        Rebalance provider weights using recent llm_usage statistics stored in the DB.

        - Sensors:
          * lookback_seconds: the lookback window (in seconds) to check usage
          * low_usage_threshold: below this usage ratio (usage / rate_limit) we may add weight
          * high_usage_threshold: above this we reduce weight
          * max_weight: upper bound for per-provider weight growth

        Usage:
          Call periodically to adapt to provider usage / quotas dynamically, then the
          next analyze_batch() call will use an updated weighted provider distribution.
        """
        try:
            now_epoch = now_ts()
            # For each client, compute usage ratio and adjust weight conservatively.
            for client in self.clients:
                try:
                    rate_limit = int(getattr(client.provider_cfg, "rate_limit_per_minute", 1) or 1)
                    recent_usage = self.db.get_llm_usage(provider=client.name(), since_ts=now_epoch - lookback_seconds)
                    usage_count = len(recent_usage)
                    ratio = float(usage_count) / float(rate_limit) if rate_limit > 0 else 0.0
                    old_weight = int(getattr(client.provider_cfg, "weight", 1) or 1)

                    if ratio > high_usage_threshold and old_weight > 1:
                        # Reduce weight conservatively by 1 step (floor at 1)
                        new_weight = max(1, old_weight - 1)
                        client.provider_cfg.weight = new_weight
                        logger.info(
                            "LLMPool.adapt_weights: provider=%s usage=%s/%s (%.2f%%) -> reducing weight %s -> %s",
                            client.name(),
                            usage_count,
                            rate_limit,
                            ratio * 100.0,
                            old_weight,
                            new_weight,
                        )
                    elif ratio < low_usage_threshold:
                        # Increase weight modestly up to max_weight
                        new_weight = min(max_weight, old_weight + 1)
                        if new_weight != old_weight:
                            client.provider_cfg.weight = new_weight
                            logger.info(
                                "LLMPool.adapt_weights: provider=%s usage=%s/%s (%.2f%%) -> increasing weight %s -> %s",
                                client.name(),
                                usage_count,
                                rate_limit,
                                ratio * 100.0,
                                old_weight,
                                new_weight,
                            )
                    else:
                        # Minor debug trace when usage is healthy
                        logger.debug(
                            "LLMPool.adapt_weights: provider=%s usage=%s/%s (%.2f%%) weight=%s (no change)",
                            client.name(),
                            usage_count,
                            rate_limit,
                            ratio * 100.0,
                            old_weight,
                        )
                except Exception:
                    # Isolation: adapt weights should not fail the whole process
                    logger.exception("LLMPool.adapt_weights: provider iteration failed for %s", client.name())

            # Rebuild the expanded list using the new weight assignment
            self.expand_providers = self._build_weighted_provider_list(self.clients)
        except Exception:
            logger.exception("LLMPool.adapt_weights: failed to adapt provider weights")


    def _choose_provider_for_item(self, idx: int) -> LLMClient:
        # Round-robin across the expanded list to get weighted distribution
        if not self.expand_providers:
            raise RuntimeError("No LLM providers available")
        return self.expand_providers[idx % len(self.expand_providers)]

    def analyze_batch(self, news_items: List[NewsItem], timeout_seconds: Optional[int] = 60) -> List[LLMSummary]:
        """
        Analyze a list of `NewsItem` objects, returning a `LLMSummary` for each item
        in the same order. If a provider fails for an item, try other providers as
        a fallback until successful (or all providers exhausted).
        """
        if not news_items:
            return []

        # Prepare a structure to keep results in original order
        results_by_idx: Dict[int, Optional[LLMSummary]] = {i: None for i in range(len(news_items))}
        # Build assignment of news -> provider indexes (round-robin with weights)
        assignment: Dict[int, List[int]] = {}  # provider_client_index -> list of news indices
        for i, item in enumerate(news_items):
            provider_client = self._choose_provider_for_item(i)
            idx = self.clients.index(provider_client)
            assignment.setdefault(idx, []).append(i)

        # For concurrency, we'll submit a worker per provider which processes their assigned news indices serially.
        futures = []
        client_idx_to_future = {}
        for client_idx, indices in assignment.items():
            client = self.clients[client_idx]
            # Submit a task to execute those indices
            future = self.executor.submit(self._process_indices_with_client, client, news_items, indices)
            futures.append(future)
            client_idx_to_future[client_idx] = future

        deadline = time.time() + (timeout_seconds or 60)
        # Wait for futures to be done, collect results as they arrive
        for future in as_completed(futures, timeout=timeout_seconds):
            try:
                client_results: List[Tuple[int, Optional[LLMSummary]]] = future.result()
                # client_results is a list of (index, LLMSummary or None if failed)
                for idx, summary in client_results:
                    results_by_idx[idx] = summary
            except Exception:
                logger.exception("Error while collecting LLM future result")

        # For any indices still None (not handled by assigned provider due to failures), we try fallback:
        remaining_indices = [i for i, v in results_by_idx.items() if v is None]
        if remaining_indices:
            logger.debug("Remaining indices unprocessed by assigned providers: %s", remaining_indices)
            # We'll attempt a sequential fallback across other providers for each remaining item
            for idx in remaining_indices:
                item = news_items[idx]
                summary = self._attempt_fallback_for_item(idx, item, exclude_clients=[])
                results_by_idx[idx] = summary

        # Finally, return the results in input order; any still None becomes a neutral summary
        out: List[LLMSummary] = []
        for i, item in enumerate(news_items):
            s = results_by_idx.get(i)
            if s is None:
                # Build neutral fallback
                s = LLMSummary(provider="none", news_id=item.published_at, direction="neutral", impact_score=0.0, confidence=0.0, summary="Failed to analyze", recommended_leverage=None, raw=None)
            out.append(s)
        return out

    def _process_indices_with_client(self, client: LLMClient, news_items: List[NewsItem], indices: List[int]) -> List[Tuple[int, Optional[LLMSummary]]]:
        """
        Worker routine for a client: process each news index assigned serially and return pairs (idx, summary).
        If a given analyze attempt fails, we return None for that index (caller may fallback).
        """
        results: List[Tuple[int, Optional[LLMSummary]]] = []
        for idx in indices:
            item = news_items[idx]
            try:
                # Attempt to analyze; note analyze() will honor rate limits
                summary = client.analyze(item)
                results.append((idx, summary))
            except Exception:
                logger.exception("LLM client %s failed analyzing news idx=%s title=%s", client.name(), idx, item.title)
                results.append((idx, None))
        return results

    def _attempt_fallback_for_item(self, idx: int, item: NewsItem, exclude_clients: Iterable[LLMClient]) -> Optional[LLMSummary]:
        """
        Try remaining providers in a deterministic order until one succeeds or all fail.
        Exclude providers in `exclude_clients` if necessary (e.g., initial provider already failed).
        """
        for client in self.clients:
            if client in exclude_clients:
                continue
            try:
                summary = client.analyze(item)
                return summary
            except Exception:
                logger.debug("Fallback provider %s failed for idx %s", client.name(), idx)
                continue
        logger.warning("All providers failed for news idx=%s; returning None", idx)
        return None

    def analyze_and_store(self, news_items: List[NewsItem], only_store_confidence_min: float = 0.0) -> List[Tuple[LLMSummary, Optional[int]]]:
        """
        Convenience method: analyze items and insert `analysis` rows into the DB for
        those analyses that meet the confidence threshold.

        Returns:
            A list of tuples (LLMSummary, analysis_id), where `analysis_id` is the
            ID of the created `analysis` DB row if the analysis was stored (confidence >= threshold),
            otherwise None.

        Note:
            This intentionally returns both the normalized LLM summary and the stored analysis
            row ID, so higher-level code (e.g., signal generators) can attach analysis IDs to
            created signals for traceability.
        """
        summaries = self.analyze_batch(news_items)
        results: List[Tuple[LLMSummary, Optional[int]]] = []
        for summary, item in zip(summaries, news_items):
            analysis_id: Optional[int] = None
            try:
                # Save analysis in the database for future deduping/insight
                created_at_ts = now_ts()
                analysis_json = {
                    "provider": summary.provider,
                    "asset": summary.asset,
                    "direction": summary.direction,
                    "impact_score": summary.impact_score,
                    "confidence": summary.confidence,
                    "summary": summary.summary,
                    "recommended_leverage": summary.recommended_leverage,
                    "raw": summary.raw,
                }
                analysis_item = AnalysisItem(
                    news_id=self._get_news_id(item),
                    provider=summary.provider,
                    analysis_json=analysis_json,
                    confidence=summary.confidence,
                    created_at=created_at_ts,
                )
                if summary.confidence >= only_store_confidence_min:
                    # store and capture the analysis id for traceability in higher-level flows
                    analysis_id = self.db.insert_analysis(analysis_item)
            except Exception:
                logger.exception("Failed to store analysis for news %s", item.title)
            results.append((summary, analysis_id))
        return results

    def _get_news_id(self, item: NewsItem) -> int:
        """
        Quickor - try to look up news in the DB by hash or published_at. If not present, insert it.
        This helper ensures we can attach analysis to a news row.
        """
        try:
            # We attempt to compute the hash the DB uses:
            base = (item.url or "") + "|" + (item.title or "") + "|" + str(item.published_at or "")
            import hashlib
            item.hash = hashlib.sha256(base.encode("utf-8")).hexdigest()
            existing = self.db.get_news_by_hash(item.hash)
            if existing:
                return int(existing["id"])
        except Exception:
            pass
        # Fallback: insert the news item
        return self.db.insert_news(item)


# Example usage (module-level test)
if __name__ == "__main__":
    logging.basicConfig(level=logging.DEBUG)
    cfg = CONFIG
    db = get_default_db(cfg.database_path)
    pool = LLMPool(config=cfg, db=db)

    # Create example news items
    sample_news = [
        NewsItem(provider="rss", url="https://example.com/1", title="Bitcoin surges after ETF approval", content="Bitcoin price jumps following ETF approval", published_at=now_ts(), fetched_at=now_ts()),
        NewsItem(provider="rss", url="https://example.com/2", title="Fed hints at another rate hike", content="A hawkish tone from the Fed spooks equity markets", published_at=now_ts(), fetched_at=now_ts()),
    ]
    analysis_pairs = pool.analyze_and_store(sample_news)
    for summary, analysis_id in analysis_pairs:
        logger.info("Analysis id=%s Summary for news: provider=%s direction=%s confidence=%.2f impact=%.2f", str(analysis_id), summary.provider, summary.direction, summary.confidence, summary.impact_score)
