BEGIN;

CREATE TABLE IF NOT EXISTS tenants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, slug),
  UNIQUE (id, organization_id)
);

CREATE TABLE IF NOT EXISTS storage_pools (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  tenant_id UUID,
  quota_bytes BIGINT NOT NULL CHECK (quota_bytes >= 0),
  used_bytes BIGINT NOT NULL DEFAULT 0 CHECK (used_bytes >= 0),
  reserved_bytes BIGINT NOT NULL DEFAULT 0 CHECK (reserved_bytes >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS libraries (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  tenant_id UUID,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, name),
  UNIQUE (id, organization_id),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS storage_pools_org_shared_unique ON storage_pools (organization_id) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS storage_pools_tenant_unique ON storage_pools (organization_id, tenant_id) WHERE tenant_id IS NOT NULL;

ALTER TABLE assets ADD COLUMN IF NOT EXISTS library_id UUID;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS source_object_key TEXT;
ALTER TABLE assets ADD COLUMN IF NOT EXISTS storage_bytes BIGINT NOT NULL DEFAULT 0;
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'assets_library_org_fk') THEN
    ALTER TABLE assets ADD CONSTRAINT assets_library_org_fk FOREIGN KEY (library_id, organization_id) REFERENCES libraries(id, organization_id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS storage_objects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  tenant_id UUID,
  library_id UUID NOT NULL,
  asset_id UUID REFERENCES assets(id) ON DELETE SET NULL,
  bucket TEXT NOT NULL,
  object_key TEXT NOT NULL UNIQUE,
  content_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  expected_size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (expected_size_bytes >= 0),
  checksum_sha256 TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('uploading','uploaded','processing','ready','failed','deleted')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE,
  FOREIGN KEY (library_id, organization_id) REFERENCES libraries(id, organization_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  tenant_id UUID,
  asset_id UUID NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
  source_object_id UUID NOT NULL REFERENCES storage_objects(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('queued','processing','completed','failed')) DEFAULT 'queued',
  attempts INTEGER NOT NULL DEFAULT 0,
  progress_percent SMALLINT NOT NULL DEFAULT 0 CHECK (progress_percent BETWEEN 0 AND 100),
  error_message TEXT NOT NULL DEFAULT '',
  locked_at TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, organization_id) REFERENCES tenants(id, organization_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS media_jobs_pending_idx ON media_jobs(status, created_at);
CREATE INDEX IF NOT EXISTS storage_objects_library_idx ON storage_objects(organization_id, library_id, status);
CREATE INDEX IF NOT EXISTS assets_library_idx ON assets(organization_id, library_id);

COMMIT;
