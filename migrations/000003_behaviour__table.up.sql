-- =====================================================================
-- 000003_behavior.up.sql
-- Camada 3 — Behavioral Governance
-- =====================================================================

-- 1. RAW EVENT — fonte da verdade. O vetor é derivado daqui.
CREATE TABLE behavior_event (
    id            BIGSERIAL    PRIMARY KEY,
    contract_id   TEXT          NOT NULL,
    agent_id      TEXT          NOT NULL,
    tool          TEXT          NOT NULL,
    payload_hash  TEXT          NOT NULL,   -- hash, nunca o payload (privacidade + detecção de repetição)
    denied        BOOLEAN       NOT NULL DEFAULT FALSE,
    ts            TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- índice que torna a query de janela rápida (WHERE contract+agent AND ts > ...)
CREATE INDEX idx_behavior_event_window
    ON behavior_event (contract_id, agent_id, ts DESC);


-- 2. BASELINE — μ, σ via Welford (n, mu, m2), por (contrato, agente, tipo).
--    'long'  = congelado após homologação (referência de drift)
--    'short' = móvel, últimas N execuções (estado recente)
CREATE TABLE behavior_baseline (
    contract_id   TEXT          NOT NULL,
    agent_id      TEXT          NOT NULL,
    kind          TEXT          NOT NULL DEFAULT 'long',   -- 'long' | 'short'
    state         TEXT          NOT NULL DEFAULT 'LEARNING', -- 'LEARNING' | 'ACTIVE'
    n             BIGINT        NOT NULL DEFAULT 0,         -- nº de execuções acumuladas
    mu            DOUBLE PRECISION[] NOT NULL,              -- médias por feature
    m2            DOUBLE PRECISION[] NOT NULL,              -- soma dos quadrados (Welford) por feature
    frozen        BOOLEAN       NOT NULL DEFAULT FALSE,     -- true => não atualiza mais (anti-envenenamento)
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
    PRIMARY KEY (contract_id, agent_id, kind),
    CONSTRAINT behavior_baseline_kind_chk  CHECK (kind  IN ('long','short')),
    CONSTRAINT behavior_baseline_state_chk CHECK (state IN ('LEARNING','ACTIVE')),
    CONSTRAINT behavior_baseline_dims_chk  CHECK (array_length(mu,1) = array_length(m2,1))
);


-- 3. SCORE POR JANELA — risk_score de cada avaliação. Série p/ painel de BURST.
CREATE TABLE behavior_score (
    id            BIGSERIAL     PRIMARY KEY,
    contract_id   TEXT          NOT NULL,
    agent_id      TEXT          NOT NULL,
    vector        DOUBLE PRECISION[] NOT NULL,   -- [25,9,4,4,0.40,3.5] — agregado da janela
    z_vector      DOUBLE PRECISION[] NOT NULL,   -- [0.62,0.32,0,4.2,3.5,0.41]
    risk_score    DOUBLE PRECISION NOT NULL,     -- 5.5
    risk_level    TEXT          NOT NULL,        -- LOW|MEDIUM|HIGH|CRITICAL
    journal_hash  TEXT,                          -- hash do cálculo no journal da VVM (auditoria)
    ts            TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CONSTRAINT behavior_score_level_chk CHECK (risk_level IN ('LOW','MEDIUM','HIGH','CRITICAL'))
);

CREATE INDEX idx_behavior_score_series
    ON behavior_score (contract_id, agent_id, ts DESC);


-- 4. DRIFT — distância euclidiana entre μ_long e μ_short, calculada periodicamente.
--    Série p/ painel de DRIFT (degradação crônica). NÃO é o mesmo que risk_score.
CREATE TABLE behavior_drift (
    id              BIGSERIAL   PRIMARY KEY,
    contract_id     TEXT        NOT NULL,
    agent_id        TEXT        NOT NULL,
    drift_distance  DOUBLE PRECISION NOT NULL,   -- euclidiana( μ_long , μ_short )
    ts              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_behavior_drift_series
    ON behavior_drift (contract_id, agent_id, ts DESC);


-- 5. GOVERNANCE MEMORY — decisão + resultado, p/ correlação e anti-envenenamento.
CREATE TABLE governance_memory (
    id            BIGSERIAL     PRIMARY KEY,
    contract_id   TEXT          NOT NULL,
    agent_id      TEXT          NOT NULL,
    risk_score    DOUBLE PRECISION NOT NULL,
    decision      TEXT          NOT NULL,        -- ALLOW|WARN|THROTTLE|REQUIRE_APPROVAL|BLOCK|QUARANTINE
    outcome       TEXT,                          -- preenchido depois: 'legit'|'violation'|null
    ts            TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX idx_governance_memory_agent
    ON governance_memory (contract_id, agent_id, ts DESC);