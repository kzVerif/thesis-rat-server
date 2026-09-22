-- Isolated integration-test fixture, NOT a production bootstrap/migration.
-- Enrollment/auth/audit subset of the local schema. No runtime rows or secrets.
CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(50) NOT NULL UNIQUE
);
CREATE TABLE permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code varchar(100) NOT NULL UNIQUE
);
CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id),
    permission_id uuid NOT NULL REFERENCES permissions(id),
    PRIMARY KEY (role_id, permission_id)
);
INSERT INTO permissions(code) VALUES ('tokens.manage'), ('users.manage');
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username varchar(50) NOT NULL UNIQUE,
    email varchar(255),
    display_name varchar(100),
    password_hash varchar(255) NOT NULL,
    role_id uuid NOT NULL REFERENCES roles(id),
    status varchar(20) NOT NULL DEFAULT 'ACTIVE'
);
CREATE TABLE user_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash char(64) NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_activity_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE rooms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name varchar(100) NOT NULL
);
CREATE TABLE agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id uuid REFERENCES rooms(id),
    hostname varchar(255) NOT NULL,
    os_info jsonb,
    mac_address varchar(17),
    ip_address inet,
    status varchar(20) NOT NULL DEFAULT 'OFFLINE'
        CHECK (status IN ('ONLINE', 'OFFLINE', 'WARNING', 'DISABLED')),
    public_key text,
    last_seen timestamptz,
    enrolled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX agents_mac_address_unique
    ON agents (lower(btrim(mac_address))) WHERE NULLIF(btrim(mac_address),'') IS NOT NULL;
CREATE TABLE tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    token_hash char(64) NOT NULL UNIQUE,
    max_use integer CHECK (max_use IS NULL OR max_use > 0),
    used_count integer NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    expires_at timestamptz,
    is_revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action varchar(100) NOT NULL,
    target_agent_id uuid REFERENCES agents(id) ON DELETE SET NULL,
    detail jsonb,
    ip_address inet,
    created_at timestamptz NOT NULL DEFAULT now()
);
