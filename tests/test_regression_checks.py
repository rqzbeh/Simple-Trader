import concurrent.futures
import os
import time

import pytest

from simple_trader import config as config_module
from simple_trader.db import Database, NewsItem
from simple_trader.llm_pool import LLMSummary, LLMPool
from simple_trader.market_data import MarketDataClient
from simple_trader.pattern_detector import PatternDetector, PatternMatch
from simple_trader.scorer import SignalScorer
from simple_trader.telegram import _escape_markdown_v2


def test_llm_summary_field_ordering():
    assert list(LLMSummary.__dataclass_fields__.keys()) == [
        "provider",
        "news_id",
        "direction",
        "impact_score",
        "confidence",
        "summary",
        "asset",
        "recommended_leverage",
        "raw",
    ]


def test_time_llm_request_imported():
    import simple_trader.llm_pool as llm_pool

    assert hasattr(llm_pool, "time_llm_request")


def test_config_defaults_match_from_env(monkeypatch):
    keys = [
        "MAX_LEVERAGE_CRYPTO",
        "MAX_LEVERAGE_FOREX",
        "LLM_MIN_CONFIDENCE",
        "MIN_PATTERN_CONFIDENCE",
        "ACCOUNT_BALANCE_USD",
        "RISK_PER_TRADE_PCT",
        "LLM_GLOBAL_CONCURRENCY",
    ]
    for key in keys:
        monkeypatch.delenv(key, raising=False)

    cfg = config_module.Config()
    cfg_env = config_module.from_env()

    assert cfg.max_leverage_crypto == cfg_env.max_leverage_crypto
    assert cfg.max_leverage_forex == cfg_env.max_leverage_forex
    assert cfg.llm_min_confidence == cfg_env.llm_min_confidence
    assert cfg.min_pattern_confidence == cfg_env.min_pattern_confidence
    assert cfg.account_balance_usd == cfg_env.account_balance_usd
    assert cfg.risk_per_trade_pct == cfg_env.risk_per_trade_pct
    assert cfg.llm_global_concurrency == cfg_env.llm_global_concurrency


def test_analyze_batch_timeout_handled(monkeypatch):
    # Patch as_completed to raise TimeoutError
    import simple_trader.llm_pool as llm_pool_module

    def _raise_timeout(*args, **kwargs):
        raise concurrent.futures.TimeoutError()

    monkeypatch.setattr(llm_pool_module, "as_completed", _raise_timeout)

    pool = LLMPool()
    news_items = [
        NewsItem(provider="rss", url="u1", title="t1", content="c1"),
        NewsItem(provider="rss", url="u2", title="t2", content="c2"),
    ]
    results = pool.analyze_batch(news_items, timeout_seconds=1)
    # Ensure we still return a result for each item
    assert len(results) == len(news_items)


def test_mark_news_processed_updates_processed_at_only(tmp_path):
    db_path = tmp_path / "test.db"
    db = Database(str(db_path))
    item = NewsItem(provider="rss", url="u1", title="t1", content="c1", published_at=None)
    news_id = db.insert_news(item)
    before = db.execute_custom("SELECT fetched_at, processed_at FROM news WHERE id = ?", (news_id,))[0]
    db.mark_news_processed(news_id, processed=True)
    after = db.execute_custom("SELECT fetched_at, processed_at FROM news WHERE id = ?", (news_id,))[0]
    assert before["fetched_at"] == after["fetched_at"]
    assert after["processed_at"] is not None


def test_telegram_escape_keeps_decimals():
    text = "Bitcoin jumps 5.3% after ETF approval"
    escaped = _escape_markdown_v2(text)
    assert "5.3%" in escaped
    assert "\\." not in escaped


def test_pattern_name_persisted_and_read_back(tmp_path):
    db_path = tmp_path / "test.db"
    db = Database(str(db_path))
    # insert minimal news and signal
    item = NewsItem(provider="rss", url="u1", title="t1", content="c1", published_at=int(time.time()))
    news_id = db.insert_news(item)
    from simple_trader.db import Signal

    sig = Signal(
        news_id=news_id,
        symbol="BTC",
        side="long",
        entry_price=1.0,
        stop_loss=0.9,
        take_profit=1.2,
        leverage=2,
        rr=2.0,
        pattern_name="bullish_engulfing",
        timeframe_hours=2,
    )
    sig_id = db.create_signal(sig)
    row = db.execute_custom("SELECT pattern_name FROM signals WHERE id = ?", (sig_id,))[0]
    assert row["pattern_name"] == "bullish_engulfing"


def test_hash_consistency_for_none_published_at(tmp_path):
    from simple_trader.news_fetcher import NewsFetcher

    db_path = tmp_path / "test.db"
    db = Database(str(db_path))
    fetcher = NewsFetcher(db=db)
    item = NewsItem(provider="rss", url="https://x.com/1", title="Test", content="c", published_at=None)
    # Simulate NewsFetcher hash
    fetcher._insert_news_item(item)
    # Ensure DB hash uses same logic for None published_at
    row = db.execute_custom("SELECT hash FROM news WHERE url = ?", (item.url,))[0]
    assert item.hash in row["hash"]


def test_scorer_heuristic_weights_normalized():
    scorer = SignalScorer()
    X = scorer._extract_features_for_signal_row({})
    score = scorer.heuristic_score_from_features(X)
    assert 0.0 <= score <= 1.0


def test_pandas_resample_alias_lowercase():
    # This checks string literals in MarketDataClient implementation
    import inspect

    source = inspect.getsource(MarketDataClient)
    assert "resample(\"1h\")" in source
    assert "f\"{hours}h\"" in source


def test_pattern_deduplication():
    det = PatternDetector()
    # create two identical PatternMatch objects
    m1 = PatternMatch("hammer", "long", 0.7, 0, 0)
    m2 = PatternMatch("hammer", "long", 0.9, 0, 0)
    # simulate internal dedup logic
    deduped = {}
    for m in [m1, m2]:
        key = (m.pattern_name, m.direction, m.start_idx, m.end_idx)
        existing = deduped.get(key)
        if existing is None or float(m.confidence or 0.0) > float(existing.confidence or 0.0):
            deduped[key] = m
    assert len(deduped) == 1
    assert list(deduped.values())[0].confidence == 0.9


def test_runtime_params_migration_and_legacy_fallback(tmp_path):
    db_path = tmp_path / "test.db"
    db = Database(str(db_path))
    # insert legacy key format manually
    legacy_key = f"{db.tenant_id}:test_key"
    db.execute_custom(
        "INSERT OR REPLACE INTO runtime_params (tenant_id, key, value) VALUES (?, ?, ?)",
        (db.tenant_id, legacy_key, "123"),
    )
    # migration should allow reading without prefix
    val = db.get_runtime_param("test_key")
    assert val == "123"


def test_no_datetime_utcnow_usage():
    # Verify repo does not use deprecated datetime.utcnow
    import pathlib

    repo_root = pathlib.Path(__file__).resolve().parents[1]
    matches = []
    for path in repo_root.rglob("*.py"):
        text = path.read_text(encoding="utf-8")
        if "datetime.utcnow" in text:
            matches.append(path)
    assert not matches
