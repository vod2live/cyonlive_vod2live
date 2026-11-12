BEGIN;

ALTER TABLE channels ADD COLUMN IF NOT EXISTS tenant_id UUID;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'channels_tenant_org_fk') THEN
    ALTER TABLE channels ADD CONSTRAINT channels_tenant_org_fk
      FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE RESTRICT;
  END IF;
END $$;

ALTER TABLE assets ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'not_required';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN IF NOT EXISTS tenant_id UUID;

UPDATE assets SET approval_status='pending'
WHERE library_id IS NOT NULL AND approval_status='not_required';

UPDATE assets a SET tenant_id=l.tenant_id
FROM libraries l
WHERE a.library_id=l.id AND a.tenant_id IS DISTINCT FROM l.tenant_id;

DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assets_review_status_check') THEN
    ALTER TABLE assets ADD CONSTRAINT assets_review_status_check
      CHECK (approval_status IN ('not_required','pending','approved','rejected'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assets_tenant_org_fk') THEN
    ALTER TABLE assets ADD CONSTRAINT assets_tenant_org_fk
      FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE;
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS assets_review_queue_idx
  ON assets (organization_id, approval_status, created_at)
  WHERE library_id IS NOT NULL;

COMMIT;
