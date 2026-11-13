BEGIN;

CREATE INDEX IF NOT EXISTS storage_objects_abandoned_upload_idx
  ON storage_objects (status, updated_at)
  WHERE status='uploading';

COMMIT;
