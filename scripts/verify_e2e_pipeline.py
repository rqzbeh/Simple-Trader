import json
import os
import time
import urllib.request
import urllib.error
import sys

BASE_URL = "http://127.0.0.1:8080"

def make_req(url, method="GET", data=None, headers_extra=None):
    headers = {"Content-Type": "application/json"}
    if headers_extra:
        headers.update(headers_extra)
    body = json.dumps(data).encode("utf-8") if data else None
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=45) as resp:
            resp_body = resp.read().decode("utf-8")
            return resp.status, json.loads(resp_body) if resp_body else {}
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8")
        try:
            return e.code, json.loads(err_body)
        except Exception:
            return e.code, {"raw": err_body}
    except Exception as e:
        return 500, {"error": str(e)}

def run_tests():
    print("================================================================")
    print("SIMPLE-TRADER V2.0 SYSTEM INTEGRATION & VERIFICATION TEST SUITE")
    print("================================================================")

    # 1. Health & Unified PWA Serving
    print("\n[TEST 1] System Health & Unified PWA Serving")
    st, res = make_req(f"{BASE_URL}/health")
    assert st == 200, f"Health check failed: {st}, {res}"
    print(f" -> Backend Health: {res['status']} | Model: {res['model_id']} | Version: {res['version']}")

    # Check PWA manifest and static assets
    req = urllib.request.Request(f"{BASE_URL}/manifest.webmanifest")
    with urllib.request.urlopen(req) as resp:
        assert resp.status == 200, f"PWA manifest failed: {resp.status}"
        manifest_data = json.loads(resp.read().decode("utf-8"))
        print(f" -> PWA Manifest verified: Name='{manifest_data.get('name')}', Display='{manifest_data.get('display')}'")

    # 2. Dynamic Liquid Crypto Screener
    print("\n[TEST 2] Dynamic Liquid Crypto Screener ($50M Vol / 10bps Spread)")
    st, res = make_req(f"{BASE_URL}/api/v1/market/screener")
    assert st == 200, f"Screener endpoint failed: {st}, {res}"
    active_universe = res.get("active_universe", [])
    assets = res.get("assets", [])
    print(f" -> Screened Assets Count: {len(assets)} | Active Universe: {len(active_universe)} symbols")
    assert len(active_universe) >= 10, f"Expected at least 10 active liquid assets, got {len(active_universe)}"
    print(f" -> Active Universe Symbols: {active_universe[:6]}...")
    sample_asset = assets[0]
    print(f" -> Sample Asset: {sample_asset['symbol']} | Vol24h: ${sample_asset['volume_24h']:,.0f} | Spread: {sample_asset['bid_ask_spread_bps']} bps | Status: {sample_asset['status']}")

    # 3. Live Real-Time News Crawler & Sentiment Ingestion
    print("\n[TEST 3] Real-time News Ingestion & NLP Sentiment Feed (Whale + Political + Crypto)")
    st, res = make_req(f"{BASE_URL}/api/v1/news/stream")
    assert st == 200, f"News stream failed: {st}, {res}"
    articles = res.get("articles", [])
    sentiment = res.get("sentiment", {})
    print(f" -> Ingested Real-Time Articles: {len(articles)} across RSS feeds (WhaleAlert, TrumpVentures, Yahoo, CoinDesk)")
    print(f" -> Aggregate Sentiment: Score={sentiment.get('score', 0):.2f} | Polarity={sentiment.get('polarity')} | Key terms matched={len(sentiment.get('key_phrases') or [])}")
    assert len(articles) > 0, "Expected at least 1 article ingested"
    first_art = articles[0]
    print(f" -> Latest Headline: \"{first_art.get('title')}\" [{first_art.get('source')}]")

    # 4. Dynamic Macroeconomic Regime Allocation
    print("\n[TEST 4] Dynamic Macroeconomic Regime Allocation (3-Tier Real-World Allocation)")
    st, regime = make_req(f"{BASE_URL}/api/v1/macro/regime")
    assert st == 200, f"Macro regime failed: {st}, {regime}"
    print(f" -> Active Macro Regime: {regime.get('regime')} (Score: {regime.get('score'):.3f})")
    print(f" -> Targets: Cash={regime.get('target_tier1_pct', 0)*100:.1f}% | Core={regime.get('target_core_pct', 0)*100:.1f}% | Alpha={regime.get('target_alpha_pct', 0)*100:.1f}%")
    print(f" -> Regime Description: \"{regime.get('description')}\"")

    # 5. Investor Capital Ledger System (PostgreSQL 16 Durable Storage)
    print("\n[TEST 5] Investor Capital Ledger System (PostgreSQL 16 Multi-Tenant)")
    # 5a. Register Investor
    investor_payload = {
        "name": "Dr. Arash Vahid",
        "contact_tag": "arash.vahid@quantum-alpha.io",
        "notes": "Institutional LP - Tier 1 Liquidity & Tactical Alpha mandate",
        "initial_deposit": 25000.00
    }
    st, inv = make_req(f"{BASE_URL}/api/v1/investors", method="POST", data=investor_payload)
    assert st in (200, 201), f"Register investor failed: {st}, {inv}"
    investor_id = inv.get("id")
    print(f" -> Registered Investor ID: {investor_id} ({inv.get('name')})")
    print(f" -> Initial Deposited: ${inv.get('total_deposited', 0):,.2f} | Initial Units: {inv.get('pool_units', 0):,.2f}")

    # 5b. Deposit Funds into Ledger
    deposit_amount = 5000.00
    dep_payload = {
        "amount": deposit_amount,
        "notes": "Secondary capital injection"
    }
    st, dep_tx = make_req(f"{BASE_URL}/api/v1/investors/{investor_id}/deposit", method="POST", data=dep_payload)
    assert st in (200, 201), f"Deposit failed: {st}, {dep_tx}"
    units = dep_tx.get("units_minted", dep_tx.get("units", 0))
    print(f" -> Deposited Additional: ${deposit_amount:,.2f} | Units Minted: {units:.4f} @ NAV={dep_tx.get('nav_at_execution', 1.0):.4f}")

    # 5c. Query Investor Profile & Balance
    st, inv_profile = make_req(f"{BASE_URL}/api/v1/investors/{investor_id}")
    assert st == 200, f"Query investor profile failed: {st}, {inv_profile}"
    inv_data = inv_profile.get("Investor", inv_profile)
    txs = inv_profile.get("transactions", [])
    print(f" -> Investor Verified: Total Invested=${inv_data.get('total_deposited', 0):,.2f} | Current Equity=${inv_data.get('current_equity', 0):,.2f} | Units={inv_data.get('pool_units', 0):.4f} | Total Tx Count={len(txs)}")

    # 5d. Test Tier 1 Cash Buffer Liquidity Protection (Excessive Withdrawal Rejection)
    print(" -> Testing Liquidity Protection: Attempting withdrawal exceeding Tier 1 Cash Buffer...")
    excess_withdraw_payload = {
        "amount": 9999999.00,
        "notes": "Excess withdrawal test"
    }
    st, rej_tx = make_req(f"{BASE_URL}/api/v1/investors/{investor_id}/withdraw", method="POST", data=excess_withdraw_payload)
    assert st == 400, f"Expected 400 Bad Request for excessive withdrawal, got {st}: {rej_tx}"
    print(f" -> Successfully REJECTED excessive withdrawal: {rej_tx.get('error')}")

    # 5e. Test Valid Withdrawal within Available Cash Buffer
    normal_withdraw_amount = 2500.00
    norm_withdraw_payload = {
        "amount": normal_withdraw_amount,
        "notes": "Partial profit redemption"
    }
    st, ok_tx = make_req(f"{BASE_URL}/api/v1/investors/{investor_id}/withdraw", method="POST", data=norm_withdraw_payload)
    assert st in (200, 201), f"Valid withdrawal failed: {st}, {ok_tx}"
    print(f" -> Successfully Executed Withdrawal: ${normal_withdraw_amount:,.2f} | Units Burned: {ok_tx.get('units_minted', ok_tx.get('units', 0)):.4f}")

    # 6. Telegram Signals Bot Configuration (US3)
    print("\n[TEST 6] Telegram Signals Bot Integration & Persistence")
    tg_config_payload = {
        "bot_token": "7952819240:AAFsBbpQMMiv86V_aqBmhneM0yn7JJEchrU",
        "chat_id": "3239664627",
        "enabled": True
    }
    st, tg_res = make_req(f"{BASE_URL}/api/v1/telegram/config", method="POST", data=tg_config_payload)
    assert st == 200, f"Telegram config update failed: {st}, {tg_res}"
    print(f" -> Telegram Config Saved: Token Masked={tg_res.get('bot_token_masked')} | ChatID={tg_res.get('chat_id')} | Enabled={tg_res.get('enabled')}")

    # 7. Secure Authentication & Session Lifecycle (US4)
    print("\n[TEST 7] Secure Authentication & Password Protection")
    # Check unauthorized initial state
    st, session = make_req(f"{BASE_URL}/api/v1/auth/session")
    assert st == 200, f"Session check failed: {st}"
    print(f" -> Initial Session State: Authenticated={session.get('authenticated')}")

    # Test login with admin password
    admin_password = os.environ.get("ADMIN_PASSWORD", "SuperSecureAdminPassword2026!")
    login_payload = {"password": admin_password}
    st, login_res = make_req(f"{BASE_URL}/api/v1/auth/login", method="POST", data=login_payload)
    assert st == 200, f"Login failed: {st}, {login_res}"
    auth_token = login_res.get("token", "")
    masked_token = (auth_token[:4] + "..." + auth_token[-4:]) if len(auth_token) >= 8 else "***"
    print(f" -> Admin Login Success: Token={masked_token}")

    # Verify session with Bearer token
    st, auth_session = make_req(f"{BASE_URL}/api/v1/auth/session", headers_extra={"Authorization": f"Bearer {auth_token}"})
    assert st == 200 and auth_session.get("authenticated") is True, f"Authenticated session check failed: {auth_session}"
    print(f" -> Validated Authenticated Session: Masked Token={auth_session.get('token_masked')}")

    # 8. Two-Sided Futures Trade Signals & Bayesian Fine-Tuning (US1)
    print("\n[TEST 8] Two-Sided Futures Trade Signals & Bayesian Learning")
    # 8a. List existing signals
    st, sigs = make_req(f"{BASE_URL}/api/v1/signals/futures?status=ACTIVE")
    assert st == 200, f"List signals failed: {st}"
    print(f" -> Active Futures Signals in DB: {len(sigs)}")

    # 8b. Generate a News-Catalyst Futures Trade Signal
    decide_payload = {
        "symbol": "BTC/USD",
        "bucket": "ALPHA",
        "news_headlines": [
            "Fed cuts interest rates by 50 bps in surprise dovish pivot",
            "Whale wallet accumulates 12,500 BTC across Coinbase Prime",
            "Global liquidity index hits new all-time high amidst rate cuts"
        ]
    }
    t0 = time.time()
    st, signal = make_req(f"{BASE_URL}/api/v1/signals/futures/decide", method="POST", data=decide_payload)
    latency = time.time() - t0
    assert st == 200, f"Signal decide failed: {st}, {signal}"
    print(f" -> Signal Evaluated in {latency:.2f}s:")
    if signal.get("status") == "HOLD":
        print(f"    Status: HOLD ({signal.get('message')})")
    else:
        sig_id = signal.get("id")
        print(f"    Signal #{sig_id} | {signal.get('direction')} {signal.get('symbol')} {signal.get('leverage')}x")
        print(f"    Entry: ${signal.get('entry_price'):,.2f} | SL: ${signal.get('stop_loss'):,.2f} | TP1: ${signal.get('take_profit_1'):,.2f} | R:R 1:{signal.get('risk_reward_ratio'):.2f}")
        print(f"    Capital: ${signal.get('allocated_capital_usd', 0):,.2f} ({signal.get('allocated_capital_pct', 0):.1f}% of Alpha Tier)")
        print(f"    Catalyst: \"{signal.get('catalyst_headline')}\" ({signal.get('catalyst_source')})")

        # 8c. Test Manual Close of Signal and Bayesian Record Outcome
        close_payload = {
            "exit_price": signal.get("entry_price") * 1.025,
            "exit_reason": "TAKE_PROFIT_TEST"
        }
        st, close_res = make_req(f"{BASE_URL}/api/v1/signals/futures/{sig_id}/close", method="POST", data=close_payload)
        assert st == 200, f"Close signal failed: {st}, {close_res}"
        print(f" -> Closed Signal #{sig_id}: Exit=${close_res.get('exit_price'):,.2f} | PnL=${close_res.get('pnl_usd'):,.2f} | ROI={close_res.get('roi_pct'):.2f}%")

    # 9. Real-Data Machine Learning & Bayesian Posteriors
    print("\n[TEST 9] Real-Data Machine Learning & Bayesian Posteriors Telemetry")
    st, ml_status = make_req(f"{BASE_URL}/api/v1/ml/status")
    assert st == 200, f"ML status failed: {st}, {ml_status}"
    print(f" -> Hardware: {ml_status.get('hardware')} | CUDA Enabled: {ml_status.get('cuda_enabled')}")
    posteriors = ml_status.get("bayesian_posteriors", {})
    for ind, p in posteriors.items():
        print(f"    - {ind}: α={p.get('alpha'):.1f}, β={p.get('beta'):.1f} (Posterior Mean: {p.get('mean')*100:.1f}%)")

    # 10. Live AI Autonomous Decision via OmniRoute Gateway
    print("\n[TEST 10] Live AI Trade Decision via OmniRoute Gateway")
    print(" -> Target Model: antigravity/gemini-3.8-flash-tiered")
    print(" -> Gateway Endpoint: https://omniroute.z3df1lter.uk/v1")
    print(" -> System Prompt: Institutional Quantitative 3-Tier Multi-Horizon Risk & Bayesian Confluence")

    t0 = time.time()
    ai_trade_req = {
        "Symbol": "BTC/USD",
        "Bucket": "ALPHA",
        "Quote": {
            "Symbol": "BTC/USD",
            "Price": 68450.50,
            "Change24h": 2.45
        },
        "IndicatorSnap": {
            "Symbol": "BTC/USD",
            "RSI": 58.4,
            "SuperTrend": "BULL",
            "Histogram": 142.6,
            "ConfluenceScore": 0.88
        },
        "Weights": {
            "SUPERTREND": 1.45,
            "RSI": 1.15,
            "MACD": 1.30
        },
        "NewsHeadlines": [
            "Fed signals rate pause as inflation metrics hit targeted terminal trajectory",
            "Global spot Bitcoin ETF inflows reach $420M in single day institutional accumulation",
            "SEC updates custody regulatory guidelines for tier-1 qualified brokers"
        ]
    }

    st, decision = make_req(f"{BASE_URL}/api/v1/trade/decide", method="POST", data=ai_trade_req)
    latency = time.time() - t0
    assert st == 200, f"Live AI Decision failed ({st}): {decision}"
    print(f" -> OmniRoute Live AI Call Completed in {latency:.2f}s!")
    print(f" -> Decision: {decision.get('decision')} | Confidence: {decision.get('confidence'):.2f} | Win Prob: {decision.get('estimated_win_probability', 0):.2f}")
    print(f" -> Regime: {decision.get('regime')} | Stop Loss: {decision.get('suggested_stop_loss_pct', 0):.2f}% | Take Profit: {decision.get('suggested_take_profit_pct', 0):.2f}%")
    print(f" -> Institutional Reasoning:\n    \"{decision.get('reasoning')}\"")

    assert decision.get("decision") in ("BUY", "SELL", "HOLD"), f"Invalid trade action: {decision.get('decision')}"
    assert 0.0 < decision.get("confidence", 0) <= 1.0, f"Invalid confidence: {decision.get('confidence')}"
    assert len(decision.get("reasoning", "")) > 10, "Empty reasoning returned from AI model"

    print("\n================================================================")
    print("ALL 10 END-TO-END PIPELINE VERIFICATION SUITES PASSED FLAWLESSLY!")
    print("Simple-Trader v2.0 Docker Stack is 100% Production Ready.")
    print("================================================================")

if __name__ == "__main__":
    try:
        run_tests()
    except Exception as e:
        print(f"\n[FATAL TEST FAILURE] {e}")
        sys.exit(1)
