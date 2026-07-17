-- =====================================================================
-- 000005_agent_info.up.sql
-- Projeção do AgentInfo (parte do artifact imutável) no Postgres.
-- A linha de contract_agents é ESCRITA UMA VEZ no deploy e nunca recebe
-- UPDATE: agent_hash é content-addressed, então config diferente => hash
-- diferente => linha nova. O .snxb segue sendo a fonte canônica.
--
-- Conteúdo .md embarcado (system_prompt, skills.content) fica FORA da
-- config inline: system_prompt em coluna TEXT própria; skills em tabela
-- filha. behavior JSONB guarda só escalares pequenos.
-- =====================================================================

ALTER TABLE contract_agents
  ADD COLUMN purpose       TEXT,
  ADD COLUMN model         JSONB,   -- { provider, name, temperature, max_tokens }
  ADD COLUMN behavior      JSONB,   -- { max_steps, on_deny, on_error }
  ADD COLUMN system_prompt TEXT;    -- .md embarcado (mesmo tratamento de skills.content)

CREATE TABLE agent_skills (
  id          SERIAL PRIMARY KEY,
  agent_hash  CHAR(66) NOT NULL,
  name        VARCHAR(255) NOT NULL,
  content     TEXT NOT NULL,                 -- .md embarcado da skill
  uses        JSONB NOT NULL DEFAULT '[]',

  CONSTRAINT fk_agent
    FOREIGN KEY (agent_hash)
    REFERENCES contract_agents(_hash)
    ON DELETE CASCADE,

  CONSTRAINT unique_agent_skill
    UNIQUE (agent_hash, name)
);
