CREATE TABLE IF NOT EXISTS shadow_decisions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    judgment_type TEXT NOT NULL CHECK (judgment_type IN ('entry','exit','news')),
    cycle_id TEXT NOT NULL,
    symbol TEXT,
    news_cluster_id BIGINT,
    state_ref TEXT,
    judge TEXT NOT NULL CHECK (judge IN ('jev','llm')),
    choice TEXT,
    noul DOUBLE PRECISION,
    score DOUBLE PRECISION,
    probabilities JSONB,
    confidence DOUBLE PRECISION,
    baseline_choice TEXT,
    latency_ms INTEGER,
    input_tokens INTEGER,
    output_tokens INTEGER,
    route TEXT,
    status TEXT NOT NULL CHECK (status IN ('ok','error')),
    error TEXT,
    outcome DOUBLE PRECISION,
    CONSTRAINT error_requires_message CHECK ((status = 'error' AND error IS NOT NULL AND error <> '') OR status = 'ok'),
    CONSTRAINT entry_choice CHECK (judgment_type <> 'entry' OR choice IS NULL OR choice IN ('LONG','SHORT','NO_TRADE')),
    CONSTRAINT news_choice CHECK (judgment_type <> 'news' OR choice IS NULL OR choice IN ('BULLISH','BEARISH','NEUTRAL','MIXED'))
);
CREATE INDEX IF NOT EXISTS idx_shadow_decisions_type_created ON shadow_decisions (judgment_type, created_at);
CREATE INDEX IF NOT EXISTS idx_shadow_decisions_cycle ON shadow_decisions (cycle_id);
CREATE INDEX IF NOT EXISTS idx_shadow_decisions_status_created ON shadow_decisions (status, created_at);
CREATE INDEX IF NOT EXISTS idx_shadow_decisions_pending_outcome ON shadow_decisions (created_at) WHERE outcome IS NULL;
