# Simple-Trader\simple_trader\scorer.py
# -*- coding: utf-8 -*-
"""
SignalScorer
============
Online signal scorer for Simple-Trader.

This module provides `SignalScorer` which can be used to:
- predict a probability (score) that a generated signal will be a winner,
- update the classifier incrementally based on observed trades (online learning),
- persist and restore the model to/from disk and optionally to the DB,
- fall back gracefully to a deterministic heuristic scorer when scikit-learn isn't available.

Design notes:
- If scikit-learn is present, we use `SGDClassifier(loss='log')` with incremental `partial_fit`.
- Input features include: LLM confidence, LLM impact score, signal RR, leverage, position USD ratio,
  and a normalized ATR estimate if available.
- Graceful fallback: When sklearn is not installed, a deterministic heuristic is used based on the above features.
- Persistence: The classifier and scaler are saved as a single pickled object to disk (or DB param if used).
- The class is lightweight & defensive: it tolerates missing input data and continues returning sensible scores.
"""

from __future__ import annotations

import base64
import json
import logging
import math
import os
import pickle
import typing
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Sequence, Tuple

import numpy as np

# Optional sklearn imports
try:
    from sklearn.linear_model import SGDClassifier
    from sklearn.preprocessing import StandardScaler

    SKLEARN_AVAILABLE = True
except Exception:
    # Keep a simple fallback if sklearn is missing
    SKLEARN_AVAILABLE = False

from simple_trader.config import CONFIG, Config
from simple_trader.db import Database, get_default_db

logger = logging.getLogger("simple_trader.scorer")
logger.addHandler(logging.NullHandler())

DEFAULT_MODEL_PATH = "scorer.pkl"
DB_MODEL_PARAM_KEY = "scorer:model_b64"


@dataclass
class ScorerConfig:
    """
    Local lightweight configuration for the scorer independent of the main config.
    """

    model_path: str = DEFAULT_MODEL_PATH
    db_store_key: str = DB_MODEL_PARAM_KEY
    load_from_db: bool = False
    save_to_db: bool = False
    # Model hyperparameters (fallback or for initial creation)
    lr_alpha: float = 0.0001
    lr_eta0: float = 0.01


class SignalScorer:
    """
    Online signal scorer combining LLM & pattern signals.

    Usage:
        scorer = SignalScorer(db=db, config=CONFIG, model_path='scorer.pkl')
        prob = scorer.predict_proba_from_signal(signal_row)
        # record real world outcome
        scorer.update_from_trade(signal_row, outcome_label)
    """

    def __init__(
        self,
        db: Optional[Database] = None,
        config: Optional[Config] = None,
        model_path: Optional[str] = None,
        use_db_store: bool = True,
        market_client: Optional[Any] = None,
    ):
        self.config = config or CONFIG
        self.db = db or get_default_db(self.config.database_path)
        self.model_path = model_path or DEFAULT_MODEL_PATH
        self.use_db_store = use_db_store
        self.market_client = (
            market_client  # Optional market data client (MarketDataClient)
        )
        self._classes = np.array([0, 1], dtype=np.int64)

        self.model = None
        self.scaler = None
        self._is_trained = False

        if SKLEARN_AVAILABLE:
            # Initialize a new model (SGD with log-loss for incremental logistic regression).
            self.model = SGDClassifier(
                loss="log",
                alpha=0.0001,
                learning_rate="optimal",
                eta0=0.01,
                average=False,
                max_iter=1,
                tol=None,
                warm_start=True,
            )
            self.scaler = StandardScaler()
        else:
            self.model = None
            self.scaler = None
            logger.debug("scikit-learn not available; using fallback heuristic scorer.")

        # Try to load persisted model from DB / disk
        try:
            self.load_model()
        except Exception:
            logger.debug("No existing model loaded; starting fresh.")

    # ------------------------
    # Model persistence
    # ------------------------
    def save_model_to_disk(self, path: Optional[str] = None) -> None:
        """
        Persist model and scaler to disk using pickle.
        """
        path = path or self.model_path
        try:
            payload = {
                "model": self.model,
                "scaler": self.scaler,
                "metadata": {"sklearn_available": SKLEARN_AVAILABLE},
            }
            with open(path, "wb") as f:
                pickle.dump(payload, f)
            logger.info("Scorer model saved to disk: %s", path)
            # Optionally store in DB runtime param as base64
            if self.use_db_store and self.db is not None:
                try:
                    with open(path, "rb") as f:
                        raw = f.read()
                    b64 = base64.b64encode(raw).decode("ascii")
                    self.db.set_runtime_param(
                        DB_MODEL_PARAM_KEY, b64, "Pickled model saved in DB"
                    )
                    logger.debug(
                        "Saved scorer model as runtime param in DB (key=%s)",
                        DB_MODEL_PARAM_KEY,
                    )
                except Exception:
                    logger.exception("Failed to save model to DB runtime params")
        except Exception:
            logger.exception("Failed to save model to disk: %s", path)

    def load_model_from_disk(self, path: Optional[str] = None) -> bool:
        """
        Load model from disk. Returns True on success, False otherwise.
        """
        path = path or self.model_path
        if not os.path.exists(path):
            logger.debug("Model file not found: %s", path)
            return False
        try:
            with open(path, "rb") as f:
                payload = pickle.load(f)
            self.model = payload.get("model", self.model)
            self.scaler = payload.get("scaler", self.scaler)
            self._is_trained = bool(getattr(self.model, "coef_", None) is not None)
            logger.info("Scorer model loaded from disk: %s", path)
            return True
        except Exception:
            logger.exception("Failed to load scorer model from disk: %s", path)
            return False

    def save_model_to_db(self) -> None:
        """
        Persist the pickled model as a base64 string in runtime parameters (small store).
        """
        try:
            # Save to a temporary bytes buffer first
            payload = {"model": self.model, "scaler": self.scaler}
            raw = pickle.dumps(payload)
            b64 = base64.b64encode(raw).decode("ascii")
            self.db.set_runtime_param(
                DB_MODEL_PARAM_KEY, b64, "Pickled model saved in DB"
            )
            logger.info("Scorer model saved to DB runtime param %s", DB_MODEL_PARAM_KEY)
        except Exception:
            logger.exception("Failed to save scorer model to DB runtime param")

    def load_model_from_db(self) -> bool:
        """
        Load the pickled model from the DB runtime parameter. Returns True on success.
        """
        try:
            b64 = self.db.get_runtime_param(DB_MODEL_PARAM_KEY)
            if not b64:
                logger.debug(
                    "No model stored in DB runtime param %s", DB_MODEL_PARAM_KEY
                )
                return False
            raw = base64.b64decode(b64)
            payload = pickle.loads(raw)
            self.model = payload.get("model", self.model)
            self.scaler = payload.get("scaler", self.scaler)
            self._is_trained = bool(getattr(self.model, "coef_", None) is not None)
            logger.info(
                "Scorer model loaded from DB runtime param %s", DB_MODEL_PARAM_KEY
            )
            return True
        except Exception:
            logger.exception("Failed to load model from DB runtime param")
            return False

    def load_model(self) -> bool:
        """
        Load model from DB if configured; otherwise from disk. Returns True if model successfully loaded.
        """
        if self.use_db_store:
            ok = self.load_model_from_db() or self.load_model_from_disk()
        else:
            ok = self.load_model_from_disk()
        return ok

    # ------------------------
    # Feature extraction
    # ------------------------
    def _extract_features_for_signal_row(
        self, signal_row: Dict[str, Any]
    ) -> np.ndarray:
        """
        Given a signal DB row (mapping-like), extract a consistent feature vector.

        Features:
        - llm_confidence: float (0..1) from latest analysis if available, else 0.0
        - impact_score: normalized to 0..1 by (impact+1)/2 (original -1..1)
        - rr: risk-reward numeric value (float)
        - leverage: as-is
        - position_usd_ratio: position_size * entry_price / account_balance (0..inf)
        - atr_norm: normalized ATR / entry_price (0..inf)
        """
        # Provide fallbacks
        llm_confidence = 0.0
        impact = 0.0
        rr = 0.0
        leverage = float(signal_row.get("leverage") or 1.0)
        position_size = float(signal_row.get("position_size") or 0.0)
        entry_price = float(signal_row.get("entry_price") or 0.0)
        symbol = signal_row.get("symbol")

        # get RR from the signal
        try:
            rr = float(signal_row.get("rr") or 0.0)
        except Exception:
            rr = 0.0

        # LLM analysis confidence & impact
        try:
            # analysis_ids stored as JSON array string; but may be None
            analysis_ids = signal_row.get("analysis_ids")
            analysis_json_object = None
            if analysis_ids:
                # parse JSON list
                try:
                    if isinstance(analysis_ids, str):
                        a_ids = json.loads(analysis_ids)
                    else:
                        a_ids = list(analysis_ids)
                except Exception:
                    a_ids = None
                if a_ids and len(a_ids) > 0:
                    aid = a_ids[0]
                    # fetch analysis row by id
                    rows = self.db.execute_custom(
                        "SELECT * FROM analysis WHERE id = ? LIMIT 1", (aid,)
                    )
                    if rows:
                        rir = rows[0]
                        if rir and rir["analysis_json"]:
                            try:
                                analysis_json_object = json.loads(rir["analysis_json"])
                            except Exception:
                                analysis_json_object = rir["analysis_json"]
            if not analysis_json_object and signal_row.get("news_id"):
                # fallback: use latest analysis for the news
                ar = self.db.get_latest_analysis(int(signal_row["news_id"]))
                if ar:
                    try:
                        analysis_json_object = (
                            json.loads(ar["analysis_json"])
                            if isinstance(ar["analysis_json"], str)
                            else ar["analysis_json"]
                        )
                    except Exception:
                        analysis_json_object = None
            if analysis_json_object:
                llm_confidence = float(analysis_json_object.get("confidence") or 0.0)
                impact = float(
                    analysis_json_object.get("impact_score")
                    or analysis_json_object.get("impact")
                    or 0.0
                )
        except Exception:
            logger.exception(
                "Error while extracting analysis info for signal id=%s",
                signal_row.get("id"),
            )

        # position USD ratio
        account_bal = float(
            getattr(self.config, "account_balance_usd", 100000.0) or 100000.0
        )
        position_usd_ratio = 0.0
        try:
            position_usd_ratio = (position_size * entry_price) / max(1.0, account_bal)
        except Exception:
            position_usd_ratio = 0.0

        # ATR normalization if market client is available
        atr_norm = 0.0
        try:
            if self.market_client and symbol and entry_price and entry_price > 0:
                df = None
                try:
                    df = self.market_client.get_2h_ohlc(symbol, lookback_hours=48)
                except Exception:
                    logger.debug("Market client failed to get 2h OHLC for %s", symbol)
                if df is not None and not df.empty:
                    atr = self.market_client.compute_atr(df, period=14)
                    # normalize by entry price to remove magnitude differences
                    atr_norm = float(atr / entry_price) if entry_price else 0.0
        except Exception:
            logger.exception("Error while computing atr_norm for symbol %s", symbol)
            atr_norm = 0.0

        # Normalized features vector
        # Scale impact (-1..1) -> (0..1)
        impact_norm = (impact + 1.0) / 2.0
        # Basic feature vector
        features = np.array(
            [
                llm_confidence,
                impact_norm,
                float(rr or 0.0),
                float(leverage or 1.0),
                float(position_usd_ratio or 0.0),
                float(atr_norm or 0.0),
            ],
            dtype=float,
        ).reshape(1, -1)
        return features

    # ------------------------
    # Scoring & prediction
    # ------------------------
    def predict_proba(self, X: np.ndarray) -> float:
        """
        Return probability (0..1) of success for a vector X (1xN).
        """
        # Defensive checks
        if X is None:
            return 0.0
        try:
            # If sklearn model is available & trained, use it
            if SKLEARN_AVAILABLE and self.model:
                if self.scaler is not None:
                    try:
                        X_scaled = self.scaler.transform(X)
                    except Exception:
                        # If scaler is not fitted yet, safely use raw X and avoid crash
                        X_scaled = X
                else:
                    X_scaled = X

                # If model supports predict_proba, use it; otherwise logistic transform
                if hasattr(self.model, "predict_proba"):
                    proba = float(self.model.predict_proba(X_scaled)[0][1])
                else:
                    # fallback: decision_function -> sigmoid
                    df = float(self.model.decision_function(X_scaled)[0])
                    proba = 1.0 / (1.0 + math.exp(-df))
                # Clamp between 0..1 for safety
                return max(0.0, min(1.0, float(proba)))
        except Exception:
            logger.exception("Sklearn prediction failed; falling back to heuristic.")
        # Fallback heuristic if model isn't present or prediction fails
        return float(self.heuristic_score_from_features(X))

    def heuristic_score_from_features(self, X: np.ndarray) -> float:
        """
        Heuristic scoring function: combine LLM confidence, impact, RR, leverage, and position USD ratio
        into a final score in [0,1].
        This is robust and deterministic for environments without sklearn.
        """
        if X is None:
            return 0.0
        try:
            # X expected: [llm_conf, impact_norm, rr, leverage, pos_usd_ratio, atr_norm]
            v = np.asarray(X).reshape(-1)
            if v.size < 6:
                # pad missing features
                v = np.pad(v, (0, 6 - v.size), "constant")
            llm_conf, impact_norm, rr, leverage, pos_ratio, atr_norm = v[:6]
        except Exception:
            logger.exception("Failed to parse features for heuristic scoring")
            llm_conf = 0.0
            impact_norm = 0.5
            rr = 0.0
            leverage = 1.0
            pos_ratio = 0.0
            atr_norm = 0.0
        # Heuristic combination - weights chosen conservatively
        # More weight to LLM confidence and impact, moderate weight to RR, small weight to leverage/pos/volatility
        w_llm = 0.5
        w_impact = 0.2
        w_rr = 0.15
        w_leverage = 0.08
        w_pos = 0.04
        # rr_remap: rr -> soft saturation
        rr_score = math.tanh(rr / 3.0)
        leverage_score = 1.0 / (1.0 + math.log1p(max(0.0, leverage)))
        pos_score = 1.0 - min(1.0, pos_ratio * 2.0)
        raw = (
            w_llm * llm_conf
            + w_impact * impact_norm
            + w_rr * rr_score
            + w_leverage * leverage_score
            + w_pos * pos_score
        )
        # Map to 0..1
        return float(max(0.0, min(1.0, raw)))

    def predict_proba_from_signal_row(self, signal_row: Dict[str, Any]) -> float:
        """
        Convenience function: extract features for a signal row and predict its win probability.
        """
        X = self._extract_features_for_signal_row(signal_row)
        return self.predict_proba(X)

    # ------------------------
    # Online updates / training
    # ------------------------
    def update_from_trade(
        self,
        signal_row: Dict[str, Any],
        outcome: str,
        executed_price: Optional[float] = None,
        exit_price: Optional[float] = None,
    ) -> Optional[bool]:
        """
        Update the scorer with the result of a trade.
        outcome: 'win' or 'loss' (or others that will be mapped to 0/1).
        Returns True on successful update, False otherwise.
        """
        try:
            label = 1 if str(outcome).lower() == "win" else 0
            X = self._extract_features_for_signal_row(signal_row)
            return self.partial_fit(X, np.array([label], dtype=np.int64))
        except Exception:
            logger.exception("Failed to update scorer from trade")
            return None

    def partial_fit(self, X: np.ndarray, y: np.ndarray) -> bool:
        """
        Incrementally train the model with a batch X (n_samples x n_features) and y labels (0/1).
        """
        if not SKLEARN_AVAILABLE:
            # no-op in fallback mode
            logger.debug("Skipping partial_fit; sklearn not available")
            return False
        try:
            X = np.asarray(X)
            y = np.asarray(y).astype(int)
            if self.scaler is not None:
                try:
                    # partial fit scaler
                    self.scaler.partial_fit(X)
                    X_scaled = self.scaler.transform(X)
                except Exception:
                    # fallback: no scaling on failure
                    X_scaled = X
            else:
                X_scaled = X
            if not self._is_trained:
                # first call must include all classes
                self.model.partial_fit(X_scaled, y, classes=self._classes)
                self._is_trained = True
            else:
                self.model.partial_fit(X_scaled, y)
            return True
        except Exception:
            logger.exception("Failed to partial_fit the scorer model")
            return False

    def fit_initial(self, X: np.ndarray, y: np.ndarray) -> bool:
        """
        Initial fit (batch) when enough historical data is available.
        """
        if not SKLEARN_AVAILABLE:
            logger.debug("Skipping fit_initial; sklearn not available")
            return False
        try:
            X = np.asarray(X)
            y = np.asarray(y).astype(int)
            if self.scaler is not None:
                try:
                    self.scaler.fit(X)
                    X_scaled = self.scaler.transform(X)
                except Exception:
                    X_scaled = X
            else:
                X_scaled = X
            # call partial_fit with classes to support incremental tracking
            self.model.partial_fit(X_scaled, y, classes=self._classes)
            self._is_trained = True
            return True
        except Exception:
            logger.exception("Initial fit failed")
            return False

    def train_on_history(
        self, lookback_seconds: int = 3600 * 24 * 30
    ) -> Tuple[int, int]:
        """
        Train / update the model using historical trades from DB (last lookback_seconds).
        Returns a summary (trained_samples_count, trained_positive_count).
        """
        # Default query: join trades and signals to obtain signal attributes
        q = (
            "SELECT trades.*, signals.symbol, signals.analysis_ids, signals.rr, signals.leverage, signals.position_size, signals.entry_price "
            "FROM trades JOIN signals ON trades.signal_id = signals.id "
            "WHERE trades.created_at >= datetime('now', ?)"
        )
        try:
            rows = self.db.execute_custom(q, (f"-{int(lookback_seconds)} seconds",))
        except Exception:
            logger.exception("Failed to query trades for historical training")
            return (0, 0)
        X_list: List[np.ndarray] = []
        y_list: List[int] = []
        for row in rows:
            try:
                # row is sqlite3.Row mapping - pass it to feature extractor (expects dict-like)
                signal_row = dict(row)  # may include signal fields
                X = self._extract_features_for_signal_row(signal_row)
                outcome = str(row.get("outcome", "") or "").lower()
                label = 1 if outcome == "win" else 0
                X_list.append(X.reshape(-1))
                y_list.append(label)
            except Exception:
                logger.exception(
                    "Error extracting features from trade row id=%s", row.get("id")
                )
        if not X_list:
            logger.info("No historical trades found to train scorer")
            return (0, 0)
        X_arr = np.vstack(X_list)
        y_arr = np.array(y_list, dtype=np.int64)
        # Fit the model
        fitted = self.fit_initial(X_arr, y_arr)
        if fitted:
            logger.info("Trained scorer on %s historical trades", len(y_arr))
        # Save model
        try:
            self.save_model_to_disk(self.model_path)
            if self.use_db_store:
                self.save_model_to_db()
        except Exception:
            logger.exception("Failed to persist trained scorer model")
        return (len(y_arr), int(np.sum(y_arr)))

    # ------------------------
    # Utilities
    # ------------------------
    def is_trained(self) -> bool:
        return bool(self._is_trained)

    def get_model_debug_info(self) -> Dict[str, Any]:
        return {
            "sklearn": SKLEARN_AVAILABLE,
            "is_trained": self._is_trained,
            "model_path": self.model_path,
            "scaler_mean": getattr(self.scaler, "mean_", None).tolist()
            if hasattr(self.scaler, "mean_")
            else None,
            "model_params": getattr(self.model, "get_params", lambda: {})(),
        }
