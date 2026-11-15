BEGIN;

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE schedule_items
  ADD CONSTRAINT schedule_items_valid_interval
  CHECK (ends_at IS NULL OR ends_at > starts_at);

ALTER TABLE schedule_items
  ADD CONSTRAINT schedule_items_no_overlap
  EXCLUDE USING gist (
    channel_id WITH =,
    tstzrange(starts_at, COALESCE(ends_at, 'infinity'::timestamptz), '[)') WITH &&
  ) WHERE (enabled);

COMMIT;
