BEGIN;

CREATE TABLE IF NOT EXISTS tenant_memberships (
  tenant_id UUID NOT NULL,
  organization_id UUID NOT NULL,
  user_id UUID NOT NULL,
  role TEXT NOT NULL DEFAULT 'membro' CHECK (role IN ('administrador','editor','operador','visualizador','membro')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, user_id) REFERENCES organization_memberships(organization_id, user_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS tenant_memberships_user_idx ON tenant_memberships (organization_id, user_id);
INSERT INTO tenant_memberships (tenant_id, organization_id, user_id, role)
SELECT t.id, t.organization_id, om.user_id, 'administrador'
FROM tenants t
JOIN organization_memberships om ON om.organization_id=t.organization_id AND om.role='administrador'
ON CONFLICT (tenant_id, user_id) DO NOTHING;

ALTER TABLE media_jobs ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;
ALTER TABLE media_jobs ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts > 0);
ALTER TABLE media_jobs ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE media_jobs DROP CONSTRAINT IF EXISTS media_jobs_status_check;
ALTER TABLE media_jobs ADD CONSTRAINT media_jobs_status_check CHECK (status IN ('queued','processing','completed','failed','dead'));
CREATE INDEX IF NOT EXISTS media_jobs_lease_idx ON media_jobs (status, lease_until, created_at);

CREATE TABLE IF NOT EXISTS asset_review_history (
  id BIGSERIAL PRIMARY KEY,
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  tenant_id UUID,
  asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  reviewer_id UUID REFERENCES users(id) ON DELETE SET NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approved','rejected')),
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS asset_review_history_asset_idx ON asset_review_history (organization_id, asset_id, created_at DESC);

COMMIT;
