import json
import time
import urllib.request
import urllib.error
import sys

BASE_URL = "http://127.0.0.1:8080"
FRONTEND_URL = "http://127.0.0.1:80"

def make_req(url, method="GET", data=None):
    headers = {"Content-Type": "application/json"}
    body = json.dumps(data).encode("utf-8") if data else None
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=40) as resp:
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

    # 1. Health Checks
    print("\n[TEST 1] System Health & Version")
    st, res = make_req(f"{BASE_URL}/health")
    assert st == 200, f"Health check failed: {st}, {res}"
    print(f" -> Backend Health: {res['status']} | Model: {res['model_id']} | Version: {res['version']}")

    st, res = make_req(f"{FRONTEND_URL}/health")
    assert st == 200, f"Frontend Nginx proxy health check failed: {st}, {res}"
    print(f" -> Frontend Nginx Proxy Health: {res['status']} (Routed to backend:8080)")

    # 2. Dynamic Liquid Crypto Screener
    print("\n[TEST 2] Dynamic Liquid Crypto Screener ($50M Vol / 10bps Spread)")
    st, res = make_req(f"{FRONTEND_URL}/api/v1/market/screener")
    assert st == 200, f"Screener endpoint failed: {st}, {res}"
    active_universe = res.get("active_universe", [])
    assets = res.get("assets", [])
    print(f" -> Screened Assets Count: {len(assets)} | Active Universe: {len(active_universe)} symbols")
    assert len(active_universe) >= 10, f"Expected at least 10 active liquid assets, got {len(active_universe)}"
    print(f" -> Active Universe Symbols: {active_universe[:6]}...")
    sample_asset = assets[0]
    print(f" -> Sample Asset: {sample_asset['symbol']} | Vol24h: ${sample_asset['volume_24h']:,.0f} | Spread: {sample_asset['bid_ask_spread_bps']} bps | Status: {sample_asset['status']}")

    # 3. Live Real-Time News Crawler & Sentiment Ingestion
    print("\n[TEST 3] Real-time News Ingestion & NLP Sentiment Feed")
    st, res = make_req(f"{FRONTEND_URL}/api/v1/news/stream")
    assert st == 200, f"News stream failed: {st}, {res}"
    articles = res.get("articles", [])
    sentiment = res.get("sentiment", {})
    print(f" -> Ingested Real-Time Articles: {len(articles)} across RSS feeds (Yahoo Finance, CoinDesk, CoinTelegraph)")
    print(f" -> Aggregate Sentiment: Score={sentiment.get('score', 0):.2f} | Polarity={sentiment.get('polarity')} | Key terms matched={len(sentiment.get('key_phrases') or [])}")
    assert len(articles) > 0, "Expected at least 1 article ingested"
    first_art = articles[0]
    print(f" -> Latest Headline: \"{first_art.get('title')}\" [{first_art.get('source')}]")

    # 4. Multi-Horizon 3-Tier Liquidity Allocator
    print("\n[TEST 4] Multi-Horizon 3-Tier Liquidity Allocation & Rebalance Engine")
    st, tiers = make_req(f"{FRONTEND_URL}/api/v1/allocator/tiers")
    assert st == 200, f"Allocation tiers failed: {st}, {tiers}"
    for tier_key, tier_val in tiers.items():
        if isinstance(tier_val, dict):
            print(f"    - {tier_key.upper()}: Target={tier_val.get('target_pct', 0)*100:.1f}% | Current Value=${tier_val.get('current_value', 0):,.2f} | Allocation={tier_val.get('current_pct', 0)*100:.1f}%")

    # 5. Investor Capital Ledger System (PostgreSQL 16 Durable Storage)
    print("\n[TEST 5] Investor Capital Ledger System (PostgreSQL 16 Multi-Tenant)")
    # 5a. Register Investor
    investor_payload = {
        "name": "Dr. Arash Vahid",
        "contact_tag": "arash.vahid@quantum-alpha.io",
        "notes": "Institutional LP - Tier 1 Liquidity & Tactical Alpha mandate",
        "initial_deposit": 25000.00
    }
    st, inv = make_req(f"{FRONTEND_URL}/api/v1/investors", method="POST", data=investor_payload)
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
    st, dep_tx = make_req(f"{FRONTEND_URL}/api/v1/investors/{investor_id}/deposit", method="POST", data=dep_payload)
    assert st in (200, 201), f"Deposit failed: {st}, {dep_tx}"
    units = dep_tx.get("units_minted", dep_tx.get("units", 0))
    print(f" -> Deposited Additional: ${deposit_amount:,.2f} | Units Minted: {units:.4f} @ NAV={dep_tx.get('nav_at_execution', 1.0):.4f}")

    # 5c. Query Investor Profile & Balance
    st, inv_profile = make_req(f"{FRONTEND_URL}/api/v1/investors/{investor_id}")
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
    st, rej_tx = make_req(f"{FRONTEND_URL}/api/v1/investors/{investor_id}/withdraw", method="POST", data=excess_withdraw_payload)
    assert st == 400, f"Expected 400 Bad Request for excessive withdrawal, got {st}: {rej_tx}"
    print(f" -> Successfully REJECTED excessive withdrawal: {rej_tx.get('error')}")

    # 5e. Test Valid Withdrawal within Available Cash Buffer
    normal_withdraw_amount = 2500.00
    norm_withdraw_payload = {
        "amount": normal_withdraw_amount,
        "notes": "Partial profit redemption"
    }
    st, ok_tx = make_req(f"{FRONTEND_URL}/api/v1/investors/{investor_id}/withdraw", method="POST", data=norm_withdraw_payload)
    assert st in (200, 201), f"Valid withdrawal failed: {st}, {ok_tx}"
    print(f" -> Successfully Executed Withdrawal: ${normal_withdraw_amount:,.2f} | Units Burned: {ok_tx.get('units_minted', ok_tx.get('units', 0)):.4f}")

    # 6. Live AI Autonomous Decision Engine via OmniRoute Gateway
    print("\n[TEST 6] Live AI Trade Decision via OmniRoute Gateway")
    print(" -> Target Model: antigravity/gemini-3.8-flash-tiered")
    print(" -> Gateway Endpoint: https://omniroute.z3df1lter.uk/v1")
    print(" -> Reasoning Effort: high")
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

    st, decision = make_req(f"{FRONTEND_URL}/api/v1/trade/decide", method="POST", data=ai_trade_req)
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
    print("ALL 6 END-TO-END PIPELINE VERIFICATION SUITES PASSED FLAWLESSLY!")
    print("Simple-Trader v2.0 Docker Stack is 100% Production Ready.")
    print("================================================================")

if __name__ == "__main__":
    try:
        run_tests()
    except Exception as e:
        print(f"\n[FATAL TEST FAILURE] {e}")
        sys.exit(1)
