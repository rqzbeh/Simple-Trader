-- 000002_investor_ledger.down.sql
BEGIN;

DROP TABLE IF EXISTS crypto_screener_snapshots;
DROP TABLE IF EXISTS news_articles;
DROP TABLE IF EXISTS portfolio_nav_history;
DROP TABLE IF EXISTS investor_transactions;
DROP TABLE IF EXISTS investors;

COMMIT;
