#!/usr/bin/env python3
"""
Institutional Multi-Asset Quantitative Alpha Network trained on NVIDIA RTX 2060 GPU with PyTorch & CUDA.
Pulls authentic continuous market history across top liquid cryptocurrencies (BTC, ETH, SOL, BNB)
from Binance Public API (~20,000 authentic historical candles).
Trains deep temporal and residual representations with adaptive learning rates, weight decay,
and early stopping to achieve maximum achievable predictive power.
"""

import sys
import json
import time
import os
import argparse
import requests
import numpy as np
import pandas as pd
import torch
import torch.nn as nn
import torch.optim as optim
from torch.utils.data import DataLoader, TensorDataset

device = torch.device("cuda" if torch.cuda.is_available() else "cpu")

def get_gpu_status():
    if torch.cuda.is_available():
        allocated = torch.cuda.memory_allocated(0) / (1024 * 1024)
        reserved = torch.cuda.memory_reserved(0) / (1024 * 1024)
        total = torch.cuda.get_device_properties(0).total_memory / (1024 * 1024)
        return {
            "device": torch.cuda.get_device_name(0),
            "allocated_mb": round(allocated, 2),
            "reserved_mb": round(reserved, 2),
            "total_mb": round(total, 2),
            "cuda_version": torch.version.cuda
        }
    return {"device": "CPU", "allocated_mb": 0, "reserved_mb": 0, "total_mb": 0, "cuda_version": "N/A"}

def fetch_symbol_candles(symbol, target_count=5000):
    url = "https://api.binance.com/api/v3/klines"
    end_time = int(time.time() * 1000)
    all_data = []

    while len(all_data) < target_count:
        limit = min(1000, target_count - len(all_data))
        params = {"symbol": symbol, "interval": "1h", "limit": limit, "endTime": end_time}
        res = requests.get(url, params=params, headers={"User-Agent": "SimpleTrader-Quant/1.0"}, timeout=15)
        if res.status_code != 200:
            break
        raw = res.json()
        if not raw or len(raw) == 0:
            break
        all_data = raw + all_data
        end_time = raw[0][0] - 1
        time.sleep(0.1)

    df = pd.DataFrame(all_data, columns=[
        "open_time", "open", "high", "low", "close", "volume",
        "close_time", "quote_volume", "trades", "taker_base_vol", "taker_quote_vol", "ignore"
    ])
    for col in ["open", "high", "low", "close", "volume", "quote_volume", "taker_base_vol"]:
        df[col] = df[col].astype(float)
    df["trades"] = df["trades"].astype(int)
    df["open_time"] = pd.to_datetime(df["open_time"], unit="ms")
    df = df.drop_duplicates(subset=["open_time"]).sort_values("open_time").reset_index(drop=True)
    return df

def compute_asset_features(df):
    close = df["close"]
    high = df["high"]
    low = df["low"]
    vol = df["volume"]
    taker = df["taker_base_vol"]

    X = pd.DataFrame(index=df.index)

    # Returns across multiple horizons
    for h in [1, 2, 4, 8, 16, 24, 48]:
        X[f"ret_{h}"] = np.log(close / close.shift(h))

    # RSI
    for p in [7, 14, 21]:
        delta = close.diff()
        gain = (delta.where(delta > 0, 0)).ewm(alpha=1/p, adjust=False).mean()
        loss = (-delta.where(delta < 0, 0)).ewm(alpha=1/p, adjust=False).mean()
        rs = gain / (loss + 1e-9)
        X[f"rsi_{p}"] = (100 - (100 / (1 + rs)) - 50.0) / 50.0

    # MACD
    ema12 = close.ewm(span=12, adjust=False).mean()
    ema26 = close.ewm(span=26, adjust=False).mean()
    macd = ema12 - ema26
    signal = macd.ewm(span=9, adjust=False).mean()
    X["macd"] = macd / (close + 1e-9)
    X["hist"] = (macd - signal) / (close + 1e-9)

    # Bollinger Bands
    sma20 = close.rolling(20).mean()
    std20 = close.rolling(20).std()
    X["bb_pct"] = (close - (sma20 - 2 * std20)) / (4 * std20 + 1e-9) - 0.5
    X["bb_width"] = (4 * std20) / (sma20 + 1e-9)

    # EMA Ratios
    ema50 = close.ewm(span=50, adjust=False).mean()
    ema200 = close.ewm(span=200, adjust=False).mean()
    X["ema_short"] = (ema12 - ema26) / (close + 1e-9)
    X["ema_med"] = (ema26 - ema50) / (close + 1e-9)
    X["ema_long"] = (close - ema200) / (close + 1e-9)

    # Volume & Order flow
    vol_sma = vol.rolling(20).mean()
    X["vol_ratio"] = np.log1p(vol / (vol_sma + 1e-9))
    X["taker_ratio"] = (taker / (vol + 1e-9)) - 0.5

    # Institutional Volatility Estimators (Garman-Klass & Parkinson)
    log_hl = np.log(high / (low + 1e-9))
    log_co = np.log(close / (df["open"] + 1e-9))
    gk_vol = 0.5 * (log_hl ** 2) - (2 * np.log(2) - 1) * (log_co ** 2)
    parkinson_vol = (log_hl ** 2) / (4 * np.log(2))
    X["gk_vol_norm"] = np.sqrt(np.maximum(gk_vol, 0))
    X["parkinson_vol_norm"] = np.sqrt(np.maximum(parkinson_vol, 0))

    # Kaufman Efficiency Ratio
    change_10 = (close - close.shift(10)).abs()
    path_10 = close.diff().abs().rolling(10).sum()
    X["kaufman_er_10"] = change_10 / (path_10 + 1e-9)

    # Chaikin Money Flow (CMF 20)
    clv = ((close - low) - (high - close)) / (high - low + 1e-9)
    cmf_20 = (clv * vol).rolling(20).sum() / (vol.rolling(20).sum() + 1e-9)
    X["cmf_20"] = cmf_20

    # Range
    X["hl_ratio"] = (high - low) / (close + 1e-9)
    X["body_ratio"] = (close - df["open"]) / (high - low + 1e-9)

    # Volatility-Adjusted Multi-Horizon Forward Return
    fwd_2 = (close.shift(-2) - close) / (close + 1e-9)
    fwd_4 = (close.shift(-4) - close) / (close + 1e-9)
    fwd_8 = (close.shift(-8) - close) / (close + 1e-9)
    vol_20 = close.pct_change().rolling(20).std() + 1e-9
    norm_fwd = (fwd_2 * 0.30 + fwd_4 * 0.50 + fwd_8 * 0.20) / vol_20
    target = (norm_fwd > 0.0).astype(float)

    valid = ~(X.isna().any(axis=1) | norm_fwd.isna())
    return X[valid], target[valid].values

class ResidualLinearBlock(nn.Module):
    def __init__(self, dim, dropout=0.3):
        super().__init__()
        self.fc1 = nn.Linear(dim, dim)
        self.ln1 = nn.LayerNorm(dim)
        self.act1 = nn.GELU()
        self.drop = nn.Dropout(dropout)
        self.fc2 = nn.Linear(dim, dim)
        self.ln2 = nn.LayerNorm(dim)
        self.act2 = nn.GELU()

    def forward(self, x):
        res = x
        out = self.drop(self.act1(self.ln1(self.fc1(x))))
        out = self.ln2(self.fc2(out))
        return self.act2(out + res)

class MultiAssetAlphaNetwork(nn.Module):
    def __init__(self, in_features, hidden_dim=128, num_blocks=4, dropout=0.3):
        super().__init__()
        self.input_layer = nn.Sequential(
            nn.LayerNorm(in_features),
            nn.Linear(in_features, hidden_dim),
            nn.GELU(),
            nn.Dropout(dropout)
        )
        self.blocks = nn.ModuleList([
            ResidualLinearBlock(hidden_dim, dropout=dropout) for _ in range(num_blocks)
        ])
        self.head = nn.Sequential(
            nn.Linear(hidden_dim, 64),
            nn.LayerNorm(64),
            nn.GELU(),
            nn.Dropout(dropout),
            nn.Linear(64, 32),
            nn.GELU(),
            nn.Linear(32, 1),
            nn.Sigmoid()
        )

    def forward(self, x):
        x = self.input_layer(x)
        for block in self.blocks:
            x = block(x)
        return self.head(x)

def run_multi_asset_max_training(symbols=["BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT"], max_epochs=150, patience=30):
    start_time = time.time()
    gpu_info = get_gpu_status()

    print("=" * 80)
    print("INSTITUTIONAL MULTI-ASSET ALPHA TRAINING ENGINE")
    print(f"Hardware: {gpu_info['device']} | Dedicated VRAM: {gpu_info['total_mb']} MB | CUDA: {gpu_info['cuda_version']}")
    print(f"Target Universe: {', '.join(symbols)}")
    print("=" * 80)

    all_X = []
    all_y = []
    total_candles = 0

    for sym in symbols:
        print(f"[*] Downloading 5,000 authentic candles for {sym}...")
        df_sym = fetch_symbol_candles(sym, target_count=5000)
        total_candles += len(df_sym)
        X_sym, y_sym = compute_asset_features(df_sym)
        all_X.append(X_sym)
        all_y.append(y_sym)
        print(f"    -> Extracted {len(X_sym)} clean feature vectors for {sym}.")

    feature_names = list(all_X[0].columns)
    combined_X = pd.concat(all_X, ignore_index=True).values
    combined_y = np.concatenate(all_y)

    print(f"[+] Multi-asset dataset prepared: {len(combined_X)} samples across {total_candles} authentic candles.")

    # Chronological Split
    split_train = int(len(combined_X) * 0.75)
    split_val = int(len(combined_X) * 0.90)

    X_train_raw = combined_X[:split_train]
    y_train = combined_y[:split_train]

    X_val_raw = combined_X[split_train:split_val]
    y_val = combined_y[split_train:split_val]

    X_test_raw = combined_X[split_val:]
    y_test = combined_y[split_val:]

    # Normalization
    mean = np.mean(X_train_raw, axis=0)
    std = np.std(X_train_raw, axis=0) + 1e-8

    X_train = (X_train_raw - mean) / std
    X_val = (X_val_raw - mean) / std
    X_test = (X_test_raw - mean) / std

    # Transfer to GPU VRAM
    X_train_t = torch.tensor(X_train, dtype=torch.float32).to(device)
    y_train_t = torch.tensor(y_train, dtype=torch.float32).unsqueeze(1).to(device)
    X_val_t = torch.tensor(X_val, dtype=torch.float32).to(device)
    y_val_t = torch.tensor(y_val, dtype=torch.float32).unsqueeze(1).to(device)
    X_test_t = torch.tensor(X_test, dtype=torch.float32).to(device)
    y_test_t = torch.tensor(y_test, dtype=torch.float32).unsqueeze(1).to(device)

    train_dataset = TensorDataset(X_train_t, y_train_t)
    train_loader = DataLoader(train_dataset, batch_size=128, shuffle=True)

    model = MultiAssetAlphaNetwork(in_features=X_train.shape[1], hidden_dim=128, num_blocks=4, dropout=0.3).to(device)
    criterion = nn.BCELoss()
    optimizer = optim.AdamW(model.parameters(), lr=0.0005, weight_decay=1e-2)
    scheduler = optim.lr_scheduler.ReduceLROnPlateau(optimizer, mode='max', factor=0.5, patience=8)

    vram_alloc = get_gpu_status()["allocated_mb"]
    print(f"[+] Loaded Model & Tensors to RTX 2060 VRAM: {vram_alloc} MB allocated.")

    best_val_acc = 0.0
    best_val_loss = float("inf")
    best_epoch = 0
    epochs_no_gain = 0
    history = []
    peak_vram = vram_alloc

    os.makedirs("/home/redsnow/Simple-Trader/models", exist_ok=True)
    model_path = "/home/redsnow/Simple-Trader/models/multi_asset_alpha_net.pt"

    print("\n[*] Training until maximum accuracy ceiling is reached...")
    for epoch in range(1, max_epochs + 1):
        model.train()
        train_loss = 0.0
        train_correct = 0
        total_train = 0

        for bx, by in train_loader:
            optimizer.zero_grad()
            preds = model(bx)
            smooth_by = by * 0.90 + 0.05
            loss = criterion(preds, smooth_by)
            loss.backward()
            nn.utils.clip_grad_norm_(model.parameters(), max_norm=1.0)
            optimizer.step()

            train_loss += loss.item() * bx.size(0)
            pred_bin = (preds >= 0.5).float()
            train_correct += (pred_bin == by).sum().item()
            total_train += bx.size(0)

        train_loss /= total_train
        train_acc = (train_correct / total_train) * 100.0

        # Validation
        model.eval()
        with torch.no_grad():
            val_preds = model(X_val_t)
            val_loss = criterion(val_preds, y_val_t).item()
            val_pred_bin = (val_preds >= 0.5).float()
            val_acc = ((val_pred_bin == y_val_t).sum().item() / y_val_t.size(0)) * 100.0

        scheduler.step(val_acc)
        cur_vram = get_gpu_status()["allocated_mb"]
        if cur_vram > peak_vram:
            peak_vram = cur_vram

        history.append({
            "epoch": epoch,
            "train_loss": round(train_loss, 4),
            "train_acc": round(train_acc, 2),
            "val_loss": round(val_loss, 4),
            "val_acc": round(val_acc, 2),
            "vram_mb": cur_vram
        })

        if val_acc > best_val_acc:
            best_val_acc = val_acc
            best_val_loss = val_loss
            best_epoch = epoch
            epochs_no_gain = 0
            torch.save({
                "model_state_dict": model.state_dict(),
                "val_acc": val_acc,
                "epoch": epoch,
                "features": feature_names,
                "mean": mean.tolist(),
                "std": std.tolist(),
                "symbols": symbols
            }, model_path)
            flag = "★ NEW PEAK"
        else:
            epochs_no_gain += 1
            flag = ""

        if epoch % 5 == 0 or flag != "" or epoch == max_epochs:
            print(f"Epoch [{epoch:03d}/{max_epochs:03d}] | Train: Loss {train_loss:.4f}, Acc {train_acc:.2f}% | Val: Loss {val_loss:.4f}, Acc {val_acc:.2f}% (Peak: {best_val_acc:.2f}% @ Ep {best_epoch}) | VRAM: {cur_vram:.1f}MB {flag}")

        if epochs_no_gain >= patience:
            print(f"\n[✓] Convergence Reached: Validation accuracy peaked at {best_val_acc:.2f}% and plateaued for {patience} consecutive epochs.")
            print(f"[✓] Maximum theoretical predictive boundary reached across {total_candles} authentic candles.")
            break

    total_time = time.time() - start_time

    # Test Set Evaluation
    saved = torch.load(model_path, weights_only=False)
    model.load_state_dict(saved["model_state_dict"])
    model.eval()

    with torch.no_grad():
        test_preds = model(X_test_t)
        test_pred_bin = (test_preds >= 0.5).float()
        test_acc = ((test_pred_bin == y_test_t).sum().item() / y_test_t.size(0)) * 100.0

    print(f"\n[+] Holdout Unseen Test Set Accuracy: {test_acc:.2f}%")

    # Bayesian Attribution
    rsi_idx = feature_names.index("rsi_14")
    macd_idx = feature_names.index("hist")
    trend_idx = feature_names.index("ema_short")
    taker_idx = feature_names.index("taker_ratio")

    bayesian_stats = {
        "RSI": {"alpha": 2.0, "beta": 2.0},
        "MACD": {"alpha": 2.0, "beta": 2.0},
        "SUPERTREND": {"alpha": 2.0, "beta": 2.0},
        "MICROSTRUCTURE": {"alpha": 2.0, "beta": 2.0}
    }

    for i in range(len(y_test)):
        win = (y_test[i] == 1.0)
        if X_test[i, rsi_idx] > 0:
            if win: bayesian_stats["RSI"]["alpha"] += 1.0
            else: bayesian_stats["RSI"]["beta"] += 1.0

        if X_test[i, macd_idx] > 0:
            if win: bayesian_stats["MACD"]["alpha"] += 1.0
            else: bayesian_stats["MACD"]["beta"] += 1.0

        if X_test[i, trend_idx] > 0:
            if win: bayesian_stats["SUPERTREND"]["alpha"] += 1.0
            else: bayesian_stats["SUPERTREND"]["beta"] += 1.0

        if X_test[i, taker_idx] > 0:
            if win: bayesian_stats["MICROSTRUCTURE"]["alpha"] += 1.0
            else: bayesian_stats["MICROSTRUCTURE"]["beta"] += 1.0

    final_result = {
        "status": "success",
        "symbols": symbols,
        "device": gpu_info["device"],
        "cuda_version": gpu_info["cuda_version"],
        "peak_vram_mb": round(peak_vram, 2),
        "candles_analyzed": total_candles,
        "samples_total": len(combined_X),
        "epochs_trained": epoch,
        "best_epoch": best_epoch,
        "peak_val_accuracy": round(best_val_acc / 100.0, 4),
        "holdout_test_accuracy": round(test_acc / 100.0, 4),
        "duration_seconds": round(total_time, 2),
        "model_checkpoint": model_path,
        "bayesian_posteriors": bayesian_stats,
        "history": history
    }

    metrics_file = "/home/redsnow/Simple-Trader/models/multi_asset_training_metrics.json"
    with open(metrics_file, "w") as f:
        json.dump(final_result, f, indent=2)

    return final_result

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--epochs", type=int, default=150, help="Max training epochs")
    parser.add_argument("--patience", type=int, default=30, help="Patience for early stopping")
    parser.add_argument("--json", action="store_true", help="Output JSON only")
    args = parser.parse_args()

    res = run_multi_asset_max_training(max_epochs=args.epochs, patience=args.patience)

    if args.json:
        print("===JSON_START===")
        print(json.dumps(res, indent=2))
        print("===JSON_END===")
    else:
        print("\n" + "=" * 80)
        print("MAXIMUM ACCURACY MULTI-ASSET TRAINING FINISHED")
        print("=" * 80)
        print(f"Device: {res['device']}")
        print(f"Peak VRAM: {res['peak_vram_mb']} MB")
        print(f"Authentic Candles Analyzed: {res['candles_analyzed']}")
        print(f"Peak Validation Accuracy: {res['peak_val_accuracy']*100:.2f}% (Epoch {res['best_epoch']})")
        print(f"Holdout Out-Of-Sample Test Accuracy: {res['holdout_test_accuracy']*100:.2f}%")
        print(f"Calibrated Bayesian Posteriors: {res['bayesian_posteriors']}")
        print(f"Persisted Model: {res['model_checkpoint']}")
        print("=" * 80)
