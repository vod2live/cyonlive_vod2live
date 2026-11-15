BEGIN;

-- A fresh token is issued for every claim. Terminal transitions must present
-- the same token, preventing an expired worker from committing stale output.
ALTER TABLE media_jobs ADD COLUMN IF NOT EXISTS claim_token UUID;
CREATE INDEX IF NOT EXISTS media_jobs_claim_token_idx ON media_jobs (id, claim_token)
  WHERE status='processing';

COMMIT;
