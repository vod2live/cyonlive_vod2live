BEGIN;
-- Keep the playout tick focused on the current item instead of scanning a channel's full schedule.
CREATE INDEX IF NOT EXISTS schedule_items_playout_current_idx
  ON schedule_items (channel_id, starts_at, position)
  WHERE enabled;
COMMIT;
