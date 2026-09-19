-- Stop the old server before applying. Run once in the same database as schema.sql.
-- Legacy generic tokens are retained as revoked records, never silently promoted
-- to enrollment credentials. Legacy user_id meant an owner, NOT a proven creator.
BEGIN;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    token_hash CHAR(64) NOT NULL UNIQUE,
    max_use INTEGER,
    used_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
LOCK TABLE tokens IN ACCESS EXCLUSIVE MODE;
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS created_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS token_hash CHAR(64);
ALTER TABLE tokens ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
        WHERE table_schema=current_schema() AND table_name='tokens' AND column_name='token') THEN
        UPDATE tokens SET token_hash=encode(digest(token,'sha256'),'hex'),
            is_revoked=TRUE,updated_at=NOW();
    END IF;
END $$;
UPDATE tokens SET updated_at=created_at WHERE updated_at IS NULL;
ALTER TABLE tokens ALTER COLUMN token_hash SET NOT NULL;
ALTER TABLE tokens ALTER COLUMN updated_at SET DEFAULT NOW();
ALTER TABLE tokens ALTER COLUMN updated_at SET NOT NULL;
-- Constraints reject invalid legacy counts rather than changing their meaning.
ALTER TABLE tokens ADD CONSTRAINT tokens_enrollment_max_use_check CHECK (max_use IS NULL OR max_use > 0);
ALTER TABLE tokens ADD CONSTRAINT tokens_enrollment_used_count_check CHECK (used_count >= 0);
ALTER TABLE tokens DROP COLUMN IF EXISTS agent_id;
ALTER TABLE tokens DROP COLUMN IF EXISTS user_id;
ALTER TABLE tokens DROP COLUMN IF EXISTS token_type;
ALTER TABLE tokens DROP COLUMN IF EXISTS token;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_hash_unique ON tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_tokens_created_by ON tokens(created_by);
CREATE INDEX IF NOT EXISTS idx_tokens_created_at ON tokens(created_at DESC,id DESC);
-- This intentionally fails/rolls back if existing agents have duplicate MACs.
-- Resolve those records explicitly; this migration does not delete agents.
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_mac_unique ON agents(lower(btrim(mac_address)))
WHERE NULLIF(btrim(mac_address),'') IS NOT NULL;
INSERT INTO permissions(code,description) VALUES ('tokens.manage','Manage agent enrollment tokens')
ON CONFLICT (code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.name='ADMINISTRATOR' AND p.code='tokens.manage'
ON CONFLICT DO NOTHING;
COMMENT ON TABLE tokens IS 'Agent enrollment tokens; usage count only, no agent associations';
COMMENT ON COLUMN tokens.created_by IS 'Authenticated creator; null for unknown legacy creators or deleted users';
COMMENT ON COLUMN tokens.token_hash IS 'SHA-256 of random enrollment secret; plaintext returned only when created';
COMMENT ON COLUMN tokens.used_count IS 'Successful enrollments; migrated legacy counts may include old validation calls';
COMMIT;
