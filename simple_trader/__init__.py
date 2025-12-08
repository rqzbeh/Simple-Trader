"""
simple_trader package

High-level package initializer for the Simple-Trader project.

This file exposes a small set of convenient utilities and central metadata.
It avoids importing heavy submodules during package import to keep startup light.
"""

from __future__ import annotations

__all__ = [
    "setup_logging",
    "get_logger",
    "Config",
    "load_config",
    "Database",
    "version",
]

# Package version
version = "0.1.0"

# Minimal logging setup helper so consumers can quickly enable logging consistently
import logging
import os
from typing import Optional


def setup_logging(
    level: int = logging.INFO,
    fmt: str | None = None,
    datefmt: str | None = None,
    logfile: Optional[str] = None,
):
    """
    Configure the default logging for the library and user scripts.

    Args:
        level: Python logging level.
        fmt: Message format to use; defaults to a simple compact format.
        datefmt: Time format string.
        logfile: If provided, logs will be written to this file as well as the console.
    """
    if fmt is None:
        fmt = "%(asctime)s [%(levelname)s] %(name)s - %(message)s"

    handlers = [logging.StreamHandler()]

    if logfile:
        try:
            handlers.append(logging.FileHandler(logfile))
        except Exception:
            # Don't fail if opening a file fails; fallback to console-only logging
            pass

    logging.basicConfig(level=level, format=fmt, datefmt=datefmt, handlers=handlers)


def get_logger(name: str) -> logging.Logger:
    """Return a configured logger for the library."""
    logger = logging.getLogger(name)
    # Default to INFO if nothing else set
    if not logger.handlers:
        setup_logging()
    return logger


# Attempt to import helpful submodules if they exist. These imports are optional
# so package import doesn't fail early when some files are not present yet in dev.
Config = None
load_config = None
Database = None

try:
    # The following imports are intentionally lazy/optional and are wrapped in try/except.
    # Consumers can import these directly from their modules (e.g., `simple_trader.config`).
    from .config import Config, load_config  # type: ignore
except Exception:
    # If config is missing (e.g., during initial dev), just leave Config as None.
    pass

try:
    from .db import Database  # type: ignore
except Exception:
    pass

# Expose a short helpful string for CLI or REPL use
__doc__ = __doc__.strip() if __doc__ else ""
__all__ = tuple(__all__)
