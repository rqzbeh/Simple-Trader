# Simple-Trader/simple_trader/metrics.py
# Prometheus metrics helpers for Simple-Trader.
#
# This module defines lightweight wrappers around prometheus_client metrics so that
# the rest of the codebase can record metrics without having to guard for
# missing prometheus_client. When the client is not installed, the functions
# are no-ops so the app still runs without instrumentation.
#
# Typical usage:
#     from simple_trader.metrics import (
#         start_metrics_server, record_llm_request, record_signal_created, set_open_signals
#     )
#     start_metrics_server(port=9000)
#     record_llm_request("groq", "success", 0.23)
#     set_open_signals(3)
#     record_signal_created("BTC", "long")
#
from __future__ import annotations

import logging
import time
from contextlib import contextmanager
from typing import Optional

logger = logging.getLogger("simple_trader.metrics")
logger.addHandler(logging.NullHandler())

# Attempt to import prometheus_client. If unavailable, gracefully fallback to no-op.
try:
    from prometheus_client import (  # type: ignore
        Counter,
        Gauge,
        Histogram,
        start_http_server,
    )

    _PROM_AVAILABLE = True
except Exception:
    # fallback no-op
    Counter = None  # type: ignore
    Gauge = None  # type: ignore
    Histogram = None  # type: ignore
    start_http_server = None  # type: ignore
    _PROM_AVAILABLE = False

# Metrics objects (initialized lazily)
_llm_requests_total = None
_llm_request_latency = None
_llm_errors_total = None
_news_processed_total = None
_signals_created_total = None
_signals_closed_total = None
_open_signals_gauge = None
_trades_recorded_total = None


def init_metrics(
    namespace: Optional[str] = None, subsystem: Optional[str] = None
) -> None:
    """
    Initialize the main metrics if prometheus_client is installed.
    This is idempotent: calling multiple times is safe.

    Args:
        namespace: Optional namespace for metric names (e.g., 'simple_trader')
        subsystem: Optional subsystem for metric grouping
    """
    global _llm_requests_total, _llm_request_latency, _llm_errors_total
    global _news_processed_total, _signals_created_total, _signals_closed_total
    global _open_signals_gauge, _trades_recorded_total

    if not _PROM_AVAILABLE:
        logger.debug("prometheus_client is not installed; metrics disabled.")
        return

    # Idempotent initialization
    if _llm_requests_total is not None:
        return

    ns = namespace or "simple_trader"
    sub = subsystem or ""

    # LLM usage metrics
    _llm_requests_total = Counter(
        name=f"{ns}_llm_requests_total",
        documentation="Total number of LLM requests",
        labelnames=("provider", "status"),
    )
    _llm_request_latency = Histogram(
        name=f"{ns}_llm_request_latency_seconds",
        documentation="LLM request latency seconds",
        labelnames=("provider",),
    )
    _llm_errors_total = Counter(
        name=f"{ns}_llm_errors_total",
        documentation="LLM provider errors",
        labelnames=("provider", "error_type"),
    )

    # News processing
    _news_processed_total = Counter(
        name=f"{ns}_news_processed_total",
        documentation="Total news items processed",
    )

    # Signals & trades
    _signals_created_total = Counter(
        name=f"{ns}_signals_created_total",
        documentation="Total signals created",
        labelnames=("symbol", "side", "pattern"),
    )
    _signals_closed_total = Counter(
        name=f"{ns}_signals_closed_total",
        documentation="Total signals closed",
        labelnames=("reason", "symbol", "side"),
    )
    _open_signals_gauge = Gauge(
        name=f"{ns}_open_signals_count",
        documentation="Number of currently open signals",
    )

    _trades_recorded_total = Counter(
        name=f"{ns}_trades_recorded_total",
        documentation="Trades recorded",
        labelnames=("outcome",),
    )


def start_metrics_server(port: int = 9000, addr: str = "0.0.0.0") -> None:
    """
    Start Prometheus metrics HTTP server on the given port. If prometheus_client
    is not available this function is a no-op.

    Args:
        port: port to listen on
        addr: address to bind to (default: 0.0.0.0)
    """
    if not _PROM_AVAILABLE or start_http_server is None:
        logger.info("Metrics server not started; prometheus_client not available.")
        return
    try:
        init_metrics()
        # start_http_server is lightweight and will run in the background
        start_http_server(port, addr)
        logger.info("Prometheus metrics server started on %s:%s", addr, port)
    except Exception:
        logger.exception("Failed to start Prometheus metrics server")


def _ensure_metrics_ready() -> bool:
    if not _PROM_AVAILABLE:
        return False
    if _llm_requests_total is None:
        init_metrics()
    return _llm_requests_total is not None


def record_llm_request(
    provider: str, status: str = "success", latency_seconds: Optional[float] = None
) -> None:
    """
    Record an LLM request outcome.
    Args:
        provider: provider name (groq, cloudflare, google, mock, etc.)
        status: 'success' or 'error'
        latency_seconds: optional request duration in seconds to observe in histogram
    """
    if not _ensure_metrics_ready():
        return
    try:
        _llm_requests_total.labels(provider, status).inc()
        if latency_seconds is not None:
            _llm_request_latency.labels(provider).observe(float(latency_seconds))
    except Exception:
        logger.debug("Failed to record llm metrics", exc_info=True)


def record_llm_error(provider: str, error_type: str = "unknown") -> None:
    """
    Increment error metric for a provider.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _llm_errors_total.labels(provider, error_type).inc()
    except Exception:
        logger.debug("Failed to record llm error metric", exc_info=True)


def record_news_processed(count: int = 1) -> None:
    """
    Increment news processed counter.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _news_processed_total.inc(count)
    except Exception:
        logger.debug("Failed to record news processed metric", exc_info=True)


def record_signal_created(
    symbol: str, side: str, pattern: Optional[str] = None
) -> None:
    """
    Record that a new signal was created.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _signals_created_total.labels(
            symbol or "UNKNOWN", side or "unknown", pattern or "unknown"
        ).inc()
    except Exception:
        logger.debug("Failed to record signal created metric", exc_info=True)


def record_signal_closed(
    reason: str, symbol: Optional[str] = None, side: Optional[str] = None
) -> None:
    """
    Record closure (close, cancel, timeout, executed) for a signal.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _signals_closed_total.labels(
            reason or "unknown", symbol or "UNKNOWN", side or "unknown"
        ).inc()
    except Exception:
        logger.debug("Failed to record signal closed metric", exc_info=True)


def set_open_signals(count: int) -> None:
    """
    Set the gauge for number of open signals.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _open_signals_gauge.set(int(count))
    except Exception:
        logger.debug("Failed to set open signals gauge", exc_info=True)


def record_trade_recorded(outcome: str = "unknown") -> None:
    """
    Record that a trade outcome was recorded in the DB.
    """
    if not _ensure_metrics_ready():
        return
    try:
        _trades_recorded_total.labels(outcome or "unknown").inc()
    except Exception:
        logger.debug("Failed to record trade metric", exc_info=True)


@contextmanager
def time_llm_request(provider: str):
    """
    Context manager to time an LLM request and record metrics automatically.
    Usage:
        with time_llm_request('groq'):
            # call provider
    After the block completes, a success metric is recorded. If an exception is raised
    inside the block, an error is recorded instead (but exception still propagates).
    """
    start = time.time()
    try:
        yield
        elapsed = time.time() - start
        record_llm_request(provider, status="success", latency_seconds=elapsed)
    except Exception:
        elapsed = time.time() - start
        try:
            record_llm_request(provider, status="error", latency_seconds=elapsed)
            record_llm_error(provider, error_type="exception")
        except Exception:
            pass
        raise


__all__ = [
    "init_metrics",
    "start_metrics_server",
    "record_llm_request",
    "record_llm_error",
    "record_news_processed",
    "record_signal_created",
    "record_signal_closed",
    "set_open_signals",
    "record_trade_recorded",
    "time_llm_request",
]
