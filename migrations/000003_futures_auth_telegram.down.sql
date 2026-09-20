-- 000003_futures_auth_telegram.down.sql

BEGIN;

DROP TABLE IF EXISTS ml_training_runs;
DROP TABLE IF EXISTS macro_regimes;
DROP TABLE IF EXISTS futures_trade_signals;
DROP TABLE IF EXISTS encrypted_system_secrets;
DROP TABLE IF EXISTS admin_users;

COMMIT;
