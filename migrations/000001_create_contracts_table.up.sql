CREATE TABLE contract_agents (
  id SERIAL PRIMARY KEY,
  _hash CHAR(66) NOT NULL UNIQUE,
  name VARCHAR(255) NOT NULL,
  version VARCHAR(255) NOT NULL
); 

CREATE TABLE contract_artifacts (
  id SERIAL PRIMARY KEY,
  _hash CHAR(66) NOT NULL UNIQUE,
  bytecode BYTEA NOT NULL,
  metadata JSONB NOT NULL,
  created_at BIGINT NOT NULL,

  agent_hash CHAR(66) NOT NULL,

  CONSTRAINT fk_agent
  FOREIGN KEY (agent_hash)
  REFERENCES contract_agents(_hash)
);

CREATE TABLE contracts (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    owner VARCHAR(255) NOT NULL,
    artifact_hash CHAR(66) NOT NULL UNIQUE,
    created_at BIGINT NOT NULL,

    CONSTRAINT fk_artifact
    FOREIGN KEY (artifact_hash)
    REFERENCES contract_artifacts(_hash)
);

CREATE TABLE blocks (
  id BIGSERIAL PRIMARY KEY,

  contract_id CHAR(66) NOT NULL,
  block_index BIGINT NOT NULL,

  hash CHAR(66) NOT NULL,
  previous_hash CHAR(66) NOT NULL,
  journal_hash CHAR(66) NOT NULL,

  timestamp BIGINT NOT NULL,
  function_name TEXT NOT NULL,
  status VARCHAR(255) NOT NULL,
  failed_reason TEXT,
  signature BYTEA NOT NULL,
  journal BYTEA NOT NULL,

  context_id VARCHAR(255),

  CONSTRAINT fk_contract
    FOREIGN KEY (contract_id)
    REFERENCES contracts(artifact_hash)
    ON DELETE CASCADE,

  CONSTRAINT unique_contract_block_index
    UNIQUE (contract_id, block_index),

  CONSTRAINT unique_contract_hash
    UNIQUE (contract_id, hash)
);