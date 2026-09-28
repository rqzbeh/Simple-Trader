CREATE TABLE IF NOT EXISTS early_exit_judgments (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    cycle_id TEXT NOT NULL,
    position_id BIGINT NOT NULL,
    symbol TEXT NOT NULL,
    cluster_id BIGINT,
    cluster_headline TEXT,
    verdict TEXT CHECK (verdict IS NULL OR verdict IN ('HOLD', 'DO_NOT_HOLD')),
    noul DOUBLE PRECISION,
    confidence DOUBLE PRECISION,
    route TEXT CHECK (route IS NULL OR route IN ('jev_direct', 'escalated')),
    guards_passed BOOLEAN,
    guard_reason TEXT,
    action TEXT CHECK (action IS NULL OR action IN ('closed', 'guarded_skip', 'error')),
    telegram_sent BOOLEAN,
    telegram_error TEXT,
    close_error TEXT,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error')),
    error TEXT,
    outcome DOUBLE PRECISION,
    CONSTRAINT early_exit_error_requires_message CHECK (
        (status = 'error' AND error IS NOT NULL AND error <> '' AND verdict IS NULL)
        OR
        (status = 'ok' AND (error IS NULL OR error = ''))
    )
);

CREATE INDEX IF NOT EXISTS idx_early_exit_pos_created ON early_exit_judgments (position_id, created_at);
CREATE INDEX IF NOT EXISTS idx_early_exit_cycle ON early_exit_judgments (cycle_id);
CREATE INDEX IF NOT EXISTS idx_early_exit_created ON early_exit_judgments (created_at);
CREATE INDEX IF NOT EXISTS idx_early_exit_pending_outcome ON early_exit_judgments (created_at) WHERE outcome IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_early_exit_terminal ON early_exit_judgments (position_id, cluster_id) WHERE action IN ('closed', 'guarded_skip');
