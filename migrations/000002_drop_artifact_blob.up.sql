-- The compiled artifact (bytecode + metadata) now lives in the content store
-- as a <hash>.snxb file. contract_artifacts becomes a registry row that only
-- links the artifact hash to its agent and creation time.
ALTER TABLE contract_artifacts DROP COLUMN IF EXISTS bytecode;
ALTER TABLE contract_artifacts DROP COLUMN IF EXISTS metadata;
