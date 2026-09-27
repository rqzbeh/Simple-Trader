# Quickstart Validation Guide: Two-Sided Futures Trading, Dynamic Macro Allocation, and Telegram Signals

**Feature**: `006-two-sided-futures-intelligence`  
**Date**: 2026-09-20  

This guide provides end-to-end verification procedures to validate all functional requirements and measurable success criteria.

---

## 1. Prerequisites & Setup

Ensure the PostgreSQL and Redis containers are running, and environment variables are set:

```bash
# 1. Ensure docker compose stack is up
docker compose up -d postgres redis

# 2. Set necessary environment variables
export ADMIN_PASSWORD="<ADMIN_PASSWORD>"
export ENCRYPTION_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
export TELEGRAM_BOT_TOKEN="your-bot-token" # optional for real push or mock-tested
export TELEGRAM_CHAT_ID="-1001234567890"

# 3. Compile or run the Go backend
go run cmd/trader/main.go
```

---

## 2. Test Scenario 1: Authentication Gate & Rate Limiting (FR-001 - FR-004)

### Step 1: Attempt unauthenticated access
```bash
# Should return HTTP 401 Unauthorized
curl -i http://localhost:8080/api/v1/signals/futures
```
*Expected Result*: Status `401 Unauthorized`.

### Step 2: Login with valid credentials
```bash
AUTH_RESP=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"username\": \"admin\", \"password\": \"$ADMIN_PASSWORD\"}")

echo $AUTH_RESP
TOKEN=$(echo $AUTH_RESP | jq -r .token)
```
*Expected Result*: Returns JSON with `token` and `expires_in: 604800`.

### Step 3: Verify authenticated access
```bash
curl -s http://localhost:8080/api/v1/signals/futures \
  -H "Authorization: Bearer $TOKEN" | jq .
```
*Expected Result*: HTTP 200 with list of futures trade signals.

### Step 4: Test Rate Limiting
Execute 6 rapid login attempts with incorrect passwords:
```bash
for i in {1..6}; do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8080/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username": "admin", "password": "WrongPassword!"}'
done
```
*Expected Result*: The 6th attempt returns `429 Too Many Requests`.

---

## 3. Test Scenario 2: Dynamic Real-World Macro Allocation (FR-010, FR-011)

Query the dynamic macroeconomic regime endpoint:
```bash
curl -s http://localhost:8080/api/v1/allocator/macro-regime | jq .
```

*Expected Verification*:
- `stress_score` reflects current geopolitical conflict and inflation indexes.
- Target tier allocations sum to exactly 100%:
  $$\text{target\_tier1\_cash\_pct} + \text{target\_tier2\_alpha\_pct} + \text{target\_tier3\_core\_pct} = 100.0\%$$
- Under elevated stress ($S \ge 0.65$), Core Preservation expands to $\ge 55\%$, Cash Buffer expands to $\ge 20\%$, and Tactical Alpha is capped at $\le 25\%$.

---

## 4. Test Scenario 3: Two-Sided Futures Signals & News Catalyst (FR-005 - FR-009)

Trigger a futures evaluation cycle on `BTC/USDT`:
```bash
curl -s -X POST http://localhost:8080/api/v1/signals/futures/decide \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"symbol": "BTC/USDT"}' | jq .
```

*Expected Verification*:
1. **Direction**: Output is explicitly `LONG` or `SHORT` (or `HOLD` if news is neutral).
2. **News Catalyst Rationale**: `catalyst_headline` cites real breaking news or on-chain whale activity.
3. **Risk-to-Reward**: `risk_reward_ratio` $\ge 1.5$ (targeting $\ge 2.0$).
4. **Capital Allocation**: `allocated_capital_pct` $\le 2.0\%$ of portfolio equity, funded from Tier 2 Tactical Alpha.
5. **Leverage**: Explicit leverage multiplier specified ($1x - 10x$).

---

## 5. Test Scenario 4: Telegram Signals Bot & ROI Reporting (FR-014 - FR-016)

Configure and test the Telegram bot integration:
```bash
# Update Telegram credentials
curl -s -X POST http://localhost:8080/api/v1/telegram/config \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"bot_token\": \"$TELEGRAM_BOT_TOKEN\", \"chat_id\": \"$TELEGRAM_CHAT_ID\", \"enabled\": true}" | jq .

# Send a test signal notification
curl -s -X POST http://localhost:8080/api/v1/telegram/test \
  -H "Authorization: Bearer $TOKEN" | jq .
```

*Expected Verification*:
- Telegram channel receives the formatted signal card with action, entry, SL, TP, leverage, and capital allocation.
- Upon simulated position exit, Telegram receives the resolution summary with hold time, exit price, and realized leveraged ROI %.

---

## 6. Test Scenario 5: Real-Data ML Model Training (FR-012, FR-013)

Run the authentic data training pipeline:
```bash
curl -s -X POST http://localhost:8080/api/v1/ml/train \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"symbol": "BTC/USDT", "timeframe": "1h", "sample_bars": 1000}' | jq .
```

*Expected Verification*:
- Ingests $\ge 1,000$ authentic historical candles from Binance public API endpoints.
- Verifies zero reliance on synthetic/mock data generators.
- Returns calibrated weights, training loss, and out-of-sample directional accuracy.

---

## 7. Test Scenario 6: PWA Install Prompt & iOS Safari Modal (FR-017 - FR-019)

1. Open `http://localhost:8080` in Chromium:
   - Verify that the custom "Install Simple-Trader" prompt appears when `beforeinstallprompt` fires.
2. Open in iOS Safari or emulate iOS Safari user-agent:
   - Verify that the step-by-step visual walkthrough modal appears displaying the Safari Share icon and the "Add to Home Screen" instructions.
3. Launch the app in standalone mode:
   - Verify all install banners and prompts are automatically suppressed.
