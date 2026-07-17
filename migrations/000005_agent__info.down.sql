DROP TABLE IF EXISTS agent_skills;

ALTER TABLE contract_agents
  DROP COLUMN IF EXISTS purpose,
  DROP COLUMN IF EXISTS model,
  DROP COLUMN IF EXISTS behavior,
  DROP COLUMN IF EXISTS system_prompt;
