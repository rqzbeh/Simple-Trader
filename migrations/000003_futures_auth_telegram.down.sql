-- 000003_futures_auth_telegram.down.sql
-- Rollback for futures signals, dynamic macro, auth, secrets, and ML training runs

BEGIN;

DROP TABLE IF EXISTS ml_training_runs CASCADE;
DROP TABLE IF EXISTS macro_regimes CASCADE;
DROP TABLE IF EXISTS futures_trade_signals CASCADE;
DROP TABLE IF EXISTS encrypted_system_secrets CASCADE;
DROP TABLE IF EXISTS admin_users CASCADE;

COMMIT;
