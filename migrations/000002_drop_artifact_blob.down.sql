-- Re-add the blob columns (nullable: historical artifact bytes cannot be
-- recovered from the DB once moved to the content store).
ALTER TABLE contract_artifacts ADD COLUMN IF NOT EXISTS bytecode BYTEA;
ALTER TABLE contract_artifacts ADD COLUMN IF NOT EXISTS metadata JSONB;
