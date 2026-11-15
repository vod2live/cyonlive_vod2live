BEGIN;

ALTER TABLE storage_objects ADD COLUMN IF NOT EXISTS etag TEXT NOT NULL DEFAULT '';
ALTER TABLE storage_objects ADD COLUMN IF NOT EXISTS version_id TEXT NOT NULL DEFAULT '';

-- Completed managed uploads must identify the exact object generation consumed
-- by the media worker. Existing rows remain compatible and are backfilled by
-- the next controlled upload completion.
CREATE INDEX IF NOT EXISTS storage_objects_version_idx
  ON storage_objects (id, version_id)
  WHERE status IN ('uploaded', 'processing', 'ready');

COMMIT;
