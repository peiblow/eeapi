CREATE TABLE agent_tools (
  id          SERIAL PRIMARY KEY,
  agent_hash  CHAR(66) NOT NULL,
  name        VARCHAR(255) NOT NULL,
  description TEXT NOT NULL,
  steps       JSONB NOT NULL,

  CONSTRAINT fk_agent
    FOREIGN KEY (agent_hash)
    REFERENCES contract_agents(_hash)
    ON DELETE CASCADE,

  CONSTRAINT unique_agent_tool
    UNIQUE (agent_hash, name)
);
