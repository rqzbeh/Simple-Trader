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

# Optional joblib for better sklearn model persist (used by Simons-style production ML)
try:
    import joblib
    JOBLIB_AVAILABLE = True
except Exception:
    JOBLIB_AVAILABLE = False

from simple_trader.config import CONFIG, Config
from simple_trader.db import Database, get_default_db

logger = logging.getLogger("simple_trader.scorer")
logger.addHandler(logging.NullHandler())

DEFAULT_MODEL_PATH = "scorer.pkl"
DB_MODEL_PARAM_KEY = "scorer:model_b64"


class CauseWeightPersister:
    """
    Persists and applies a small cause -> weight mapping (Simons: statistical factors from every outcome;
    Buffett: penalize repeating violations of margin-of-safety like insufficient hedge).
    Weights are loaded from DB cause_weights table (or runtime_param fallback).
    Applied as additive penalty to base score or as extra feature bias.
    Small mapping: e.g. 'insufficient_hedge': -0.25 after several bad politician-disclosure alpha trades without hedge.
    """
    def __init__(self, db: Optional[Database] = None):
        self.db = db or get_default_db()
        self.weights: Dict[str, float] = {}
        self.load()

    def load(self):
        try:
            if self.db:
                self.weights = self.db.get_cause_weights() or {}
            if not self.weights:
                # Fallback to runtime param JSON
                raw = self.db.get_runtime_param("ml_cause_weights") if self.db else None
                if raw:
                    self.weights = json.loads(raw)
        except Exception:
            self.weights = {}
        logger.debug("CauseWeightPersister loaded %d causes", len(self.weights))

    def save(self):
        # Already persisted via db.update_cause_weight in log_regret; here ensure runtime sync
        try:
            if self.db and self.weights:
                self.db.set_runtime_param("ml_cause_weights", json.dumps(self.weights), "Cause->weight ML feedback (auto)")
        except Exception:
            pass

    def get_penalty(self, causes: List[str] | str | None) -> float:
        """Sum of weights for matched causes. Negative = penalty. Always reload for live updates from regret log."""
        self.load()  # Simons continuous: fresh from cause_weights table
        if not causes:
            return 0.0
        if isinstance(causes, str):
            try:
                causes = json.loads(causes)
            except:
                causes = [causes]
        pen = 0.0
        for c in causes:
            w = self.weights.get(c, 0.0)
            pen += w
        return pen

    def apply_to_score(self, base_score: float, causes: List[str] | None = None, signal_row: dict | None = None) -> float:
        """Return adjusted score [0,1] with cause penalties applied. Clamped."""
        pen = self.get_penalty(causes)
        # Also check audit for embedded causes
        if signal_row and not causes:
            try:
                audit = json.loads(signal_row.get("decision_audit", "{}")) if isinstance(signal_row.get("decision_audit"), str) else {}
                if "[CAUSES:" in str(audit.get("notes", "")):
                    # rough extract
                    pass
            except:
                pass
        adj = max(0.0, min(1.0, base_score + pen))  # pen is negative or small positive
        if pen != 0:
            logger.debug("CauseWeightPersister: base=%.3f pen=%.3f -> adj=%.3f", base_score, pen, adj)
        return adj

    def update(self, cause: str, delta: float, is_bad: bool = True):
        """Direct update (usually called via db.log_regret which does it)."""
        try:
            if self.db:
                self.db.update_cause_weight(cause, delta, is_bad)
            self.weights[cause] = self.weights.get(cause, 0.0) + delta
            self.save()
        except Exception:
            pass


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
        self.db = db or get_default_db(
            self.config.database_path, tenant_id=self.config.tenant_id
        )
        self.model_path = model_path or DEFAULT_MODEL_PATH
        self.use_db_store = use_db_store
        self.market_client = (
            market_client  # Optional market data client (MarketDataClient)
        )
        self._classes = np.array([0, 1], dtype=np.int64)
        self.cause_persister = CauseWeightPersister(self.db)  # Simons/Buffett cause->weight ML feedback

        self.model = None
        self.scaler = None
        self._is_trained = False

        if SKLEARN_AVAILABLE:
            # Initialize a new model (SGD with log-loss for incremental logistic regression).
            self.model = SGDClassifier(
                loss="log_loss",
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
        Persist model and scaler to disk using pickle (or joblib if sklearn model + joblib available for production robustness).
        """
        path = path or self.model_path
        try:
            if JOBLIB_AVAILABLE and SKLEARN_AVAILABLE and self.model is not None:
                payload = {"model": self.model, "scaler": self.scaler, "metadata": {"sklearn": True, "joblib": True}}
                joblib.dump(payload, path + ".joblib")
                logger.info("Scorer model saved to disk (joblib): %s.joblib", path)
                # also save pickle for compat
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
        Prefers joblib .joblib if present (for sklearn models).
        """
        path = path or self.model_path
        # Try joblib first
        if JOBLIB_AVAILABLE and os.path.exists(path + ".joblib"):
            try:
                payload = joblib.load(path + ".joblib")
                self.model = payload.get("model", self.model)
                self.scaler = payload.get("scaler", self.scaler)
                self._is_trained = bool(getattr(self.model, "coef_", None) is not None) if self.model else False
                logger.info("Scorer model loaded from disk (joblib): %s.joblib", path)
                return True
            except Exception:
                logger.debug("Joblib load failed, trying pickle")
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
        Persist model parameters as safe JSON in runtime parameters.
        """
        try:
            payload = self._build_safe_db_payload()
            if payload is None:
                return
            b64 = base64.b64encode(
                json.dumps(payload, separators=(",", ":")).encode("utf-8")
            ).decode("ascii")
            self.db.set_runtime_param(
                DB_MODEL_PARAM_KEY, b64, "JSON-serialized scorer model saved in DB"
            )
            logger.info("Scorer model saved to DB runtime param %s", DB_MODEL_PARAM_KEY)
        except Exception:
            logger.exception("Failed to save scorer model to DB runtime param")

    def load_model_from_db(self) -> bool:
        """
        Load the JSON-serialized model from DB runtime parameters. Returns True on success.
        """
        try:
            b64 = self.db.get_runtime_param(DB_MODEL_PARAM_KEY)
            if not b64:
                logger.debug(
                    "No model stored in DB runtime param %s", DB_MODEL_PARAM_KEY
                )
                return False
            raw = base64.b64decode(b64)
            payload = json.loads(raw.decode("utf-8"))
            if not self._restore_from_safe_db_payload(payload):
                return False
            logger.info(
                "Scorer model loaded from DB runtime param %s", DB_MODEL_PARAM_KEY
            )
            return True
        except Exception:
            logger.exception("Failed to load model from DB runtime param")
            return False

    def _build_safe_db_payload(self) -> Optional[Dict[str, Any]]:
        if not SKLEARN_AVAILABLE or self.model is None:
            return None
        coef = getattr(self.model, "coef_", None)
        intercept = getattr(self.model, "intercept_", None)
        classes = getattr(self.model, "classes_", None)
        if coef is None or intercept is None or classes is None:
            return None
        payload: Dict[str, Any] = {
            "schema_version": 1,
            "model": {
                "coef": np.asarray(coef, dtype=float).tolist(),
                "intercept": np.asarray(intercept, dtype=float).tolist(),
                "classes": np.asarray(classes, dtype=int).tolist(),
            },
        }
        if self.scaler is not None and hasattr(self.scaler, "mean_"):
            payload["scaler"] = {
                "mean": np.asarray(self.scaler.mean_, dtype=float).tolist(),
                "scale": np.asarray(self.scaler.scale_, dtype=float).tolist(),
                "var": np.asarray(self.scaler.var_, dtype=float).tolist(),
                "n_samples_seen": int(getattr(self.scaler, "n_samples_seen_", 1)),
            }
        return payload

    def _restore_from_safe_db_payload(self, payload: Dict[str, Any]) -> bool:
        if not SKLEARN_AVAILABLE or self.model is None:
            return False
        model_payload = payload.get("model")
        if not isinstance(model_payload, dict):
            return False
        try:
            self.model.classes_ = np.asarray(model_payload["classes"], dtype=np.int64)
            self.model.coef_ = np.asarray(model_payload["coef"], dtype=float)
            self.model.intercept_ = np.asarray(model_payload["intercept"], dtype=float)
            self.model.n_features_in_ = int(self.model.coef_.shape[1])
            self.model.t_ = float(getattr(self.model, "t_", 1.0))
            scaler_payload = payload.get("scaler")
            if (
                self.scaler is not None
                and isinstance(scaler_payload, dict)
                and "mean" in scaler_payload
                and "scale" in scaler_payload
            ):
                self.scaler.mean_ = np.asarray(scaler_payload["mean"], dtype=float)
                self.scaler.scale_ = np.asarray(scaler_payload["scale"], dtype=float)
                self.scaler.var_ = np.asarray(
                    scaler_payload.get("var", np.square(self.scaler.scale_)),
                    dtype=float,
                )
                self.scaler.n_samples_seen_ = int(
                    scaler_payload.get("n_samples_seen", 1)
                )
                self.scaler.n_features_in_ = int(len(self.scaler.mean_))
            self._is_trained = True
            return True
        except Exception:
            logger.exception("Failed to restore scorer model payload")
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

        # Rich features passed from full system (Allocator + Risk + Knowledge)
        alpha_bucket = float(signal_row.get("current_alpha_bucket_risk") or 0) / 10000.0  # scale
        hedge_ratio_f = float(signal_row.get("current_hedge_risk") or 0) / max(alpha_bucket*10000 + 1, 1)
        risk_off_f = 1.0 if signal_row.get("risk_off") else 0.0

        # New from whale/politics (Simons data edge) + Buffett value proxy
        whale_f = 1.0 if "whale" in str(signal_row.get("provider", "")).lower() or "whale" in str(signal_row.get("decision_audit", "")).lower() else 0.0
        politics_f = 1.0 if "politics" in str(signal_row.get("provider", "")).lower() or "trump" in str(signal_row.get("decision_audit", "")).lower() or "pelosi" in str(signal_row.get("decision_audit", "")).lower() else 0.0
        value_f = 1.0 if (signal_row.get("asset_class") in ("GOLD", "OIL") and risk_off_f > 0) else 0.5  # Buffett value when fear high

        # Basic + rich feature vector (12 features)
        features = np.array(
            [
                llm_confidence,
                impact_norm,
                float(rr or 0.0),
                float(leverage or 1.0),
                float(position_usd_ratio or 0.0),
                float(atr_norm or 0.0),
                alpha_bucket,
                hedge_ratio_f,
                risk_off_f,
                whale_f,
                politics_f,
                value_f,
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
        Applies CauseWeightPersister penalties (from regret_table + cause_weights) for Buffett/Simons learning:
        e.g. repeated 'insufficient_hedge' on politician disclosure alpha trades lowers future prob for similar.
        """
        X = self._extract_features_for_signal_row(signal_row)
        base = self.predict_proba(X)
        # Extract causes from notes or decision_audit for penalty
        causes = None
        notes = str(signal_row.get("notes", "") or "")
        if "[CAUSES:" in notes:
            try:
                causes = notes.split("[CAUSES:")[1].split("]")[0].split("; ")
            except:
                causes = None
        if not causes:
            try:
                audit = json.loads(signal_row.get("decision_audit") or "{}")
                if "[CAUSES:" in str(audit):
                    causes = str(audit).split("[CAUSES:")[1].split("]")[0].split("; ")
            except:
                pass
        adj = self.cause_persister.apply_to_score(base, causes=causes, signal_row=signal_row) if hasattr(self, 'cause_persister') else base
        return adj

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
        except Exception as e:
            msg = str(e)
            if "mismatch" in msg.lower() or "n_features" in msg.lower() or "shape" in msg.lower():
                logger.warning("ML features updated (new regret/cause/politician/whale/value features), resetting SGD learner for compatibility (Simons regime shift handling)")
                # Reset model for new feature dim (12+ now)
                self.model = SGDClassifier(loss="log_loss", alpha=0.0001, learning_rate="optimal", eta0=0.01, max_iter=1, tol=None, warm_start=True)
                self._is_trained = False
                try:
                    self.scaler = StandardScaler()
                    self.scaler.partial_fit(X)
                except:
                    pass
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
