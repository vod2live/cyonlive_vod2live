BEGIN;

CREATE TABLE IF NOT EXISTS organizations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO organizations (name, slug)
VALUES ('Organização legada', 'legacy')
ON CONFLICT (slug) DO NOTHING;

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_unique ON users (lower(email));

CREATE TABLE IF NOT EXISTS organization_memberships (
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('administrador','operador','editor','visualizador')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, user_id),
  UNIQUE (user_id, organization_id)
);

ALTER TABLE channels ADD COLUMN IF NOT EXISTS organization_id UUID;
UPDATE channels SET organization_id = (SELECT id FROM organizations WHERE slug = 'legacy') WHERE organization_id IS NULL;
ALTER TABLE channels ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE channels ADD CONSTRAINT channels_organization_fk FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE channels DROP CONSTRAINT IF EXISTS channels_slug_key;
ALTER TABLE channels ADD CONSTRAINT channels_org_slug_unique UNIQUE (organization_id, slug);
ALTER TABLE channels ADD CONSTRAINT channels_id_org_unique UNIQUE (id, organization_id);

ALTER TABLE assets ADD COLUMN IF NOT EXISTS organization_id UUID;
UPDATE assets SET organization_id = (SELECT id FROM organizations WHERE slug = 'legacy') WHERE organization_id IS NULL;
ALTER TABLE assets ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE assets ADD CONSTRAINT assets_organization_fk FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE assets ADD CONSTRAINT assets_id_org_unique UNIQUE (id, organization_id);

ALTER TABLE schedule_items ADD COLUMN IF NOT EXISTS organization_id UUID;
UPDATE schedule_items s SET organization_id = c.organization_id FROM channels c WHERE s.channel_id = c.id AND s.organization_id IS NULL;
ALTER TABLE schedule_items ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE schedule_items ADD CONSTRAINT schedule_channel_org_fk FOREIGN KEY (channel_id, organization_id) REFERENCES channels(id, organization_id) ON DELETE CASCADE;
ALTER TABLE schedule_items ADD CONSTRAINT schedule_asset_org_fk FOREIGN KEY (asset_id, organization_id) REFERENCES assets(id, organization_id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (organization_id, name),
  UNIQUE (id, organization_id)
);

CREATE TABLE IF NOT EXISTS team_memberships (
  organization_id UUID NOT NULL,
  team_id UUID NOT NULL,
  user_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (team_id, user_id),
  FOREIGN KEY (team_id, organization_id) REFERENCES teams(id, organization_id) ON DELETE CASCADE,
  FOREIGN KEY (organization_id, user_id) REFERENCES organization_memberships(organization_id, user_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS audit_events (
  id BIGSERIAL PRIMARY KEY,
  organization_id UUID REFERENCES organizations(id) ON DELETE SET NULL,
  actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  resource TEXT NOT NULL,
  http_status SMALLINT NOT NULL,
  request_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS audit_events_org_created_idx ON audit_events (organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_events_actor_created_idx ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS channels_org_idx ON channels (organization_id);
CREATE INDEX IF NOT EXISTS assets_org_idx ON assets (organization_id);

COMMIT;
