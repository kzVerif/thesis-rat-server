CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(50) NOT NULL UNIQUE,
    description VARCHAR(255),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    code VARCHAR(100) NOT NULL UNIQUE,
    description VARCHAR(255),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO permissions (code, description)
VALUES
    ('rooms.manage', 'จัดการข้อมูลห้อง'),
    ('users.manage', 'จัดการข้อมูลผู้ใช้')
ON CONFLICT (code) DO NOTHING;
CREATE TABLE role_permissions (
    role_id UUID NOT NULL,
    permission_id UUID NOT NULL,

    PRIMARY KEY (role_id, permission_id),

    CONSTRAINT fk_role_permissions_role
        FOREIGN KEY (role_id)
        REFERENCES roles(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_role_permissions_permission
        FOREIGN KEY (permission_id)
        REFERENCES permissions(id)
        ON DELETE CASCADE
);
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(255) UNIQUE,

    password_hash VARCHAR(255) NOT NULL,

    display_name VARCHAR(100),

    role_id UUID NOT NULL,

    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE'
        CHECK (
            status IN (
                'ACTIVE',
                'DISABLED',
                'LOCKED'
            )
        ),

    failed_login_attempts INTEGER NOT NULL DEFAULT 0,

    locked_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_users_role
        FOREIGN KEY (role_id)
        REFERENCES roles(id)
);
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NOT NULL,

    token_hash CHAR(64) NOT NULL UNIQUE,

    ip_address INET,
    user_agent TEXT,

    expires_at TIMESTAMPTZ NOT NULL,

    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    revoked_at TIMESTAMPTZ,
    revoked_reason VARCHAR(255),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_user_sessions_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE CASCADE
);
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    name VARCHAR(100) NOT NULL UNIQUE,

    description VARCHAR(255),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    room_id UUID,

    hostname VARCHAR(255) NOT NULL,

    os_info JSONB,

    mac_address VARCHAR(17),
    ip_address INET,

    status VARCHAR(20) NOT NULL DEFAULT 'OFFLINE'
        CHECK (
            status IN (
                'ONLINE',
                'OFFLINE',
                'WARNING',
                'DISABLED'
            )
        ),

    last_seen TIMESTAMPTZ,

    enrolled_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_agents_room
        FOREIGN KEY (room_id)
        REFERENCES rooms(id)
        ON DELETE SET NULL
);
CREATE TABLE commands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    agent_id UUID NOT NULL,

    issued_by UUID NOT NULL,

    command_type VARCHAR(100) NOT NULL,

    payload JSONB,

    status VARCHAR(30) NOT NULL DEFAULT 'QUEUED'
        CHECK (
            status IN (
                'QUEUED',
                'DELIVERED',
                'ACKNOWLEDGED',
                'RUNNING',
                'SUCCEEDED',
                'FAILED',
                'CANCELLED',
                'EXPIRED'
            )
        ),

    result_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,

    CONSTRAINT fk_commands_agent
        FOREIGN KEY (agent_id)
        REFERENCES agents(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_commands_user
        FOREIGN KEY (issued_by)
        REFERENCES users(id)
        ON DELETE RESTRICT
);
CREATE TABLE logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID,

    action VARCHAR(100) NOT NULL,

    target_agent_id UUID,

    detail JSONB,

    ip_address INET,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_logs_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE SET NULL,

    CONSTRAINT fk_logs_agent
        FOREIGN KEY (target_agent_id)
        REFERENCES agents(id)
        ON DELETE SET NULL
);
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    filename VARCHAR(255) NOT NULL,

    original_name VARCHAR(255) NOT NULL,

    file_size BIGINT NOT NULL
        CHECK (file_size >= 0),

    storage_path TEXT NOT NULL,

    hash_sha256 CHAR(64) NOT NULL,

    uploaded_by UUID NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_files_uploaded_by
        FOREIGN KEY (uploaded_by)
        REFERENCES users(id)
        ON DELETE RESTRICT
);
CREATE TABLE av_scan_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    agent_id UUID NOT NULL,

    command_id UUID NOT NULL,

    scan_type VARCHAR(30) NOT NULL,

    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    total_files_scanned BIGINT NOT NULL DEFAULT 0
        CHECK (total_files_scanned >= 0),

    threats_found INTEGER NOT NULL DEFAULT 0
        CHECK (threats_found >= 0),

    threat_details JSONB,

    status VARCHAR(30) NOT NULL
        CHECK (
            status IN (
                'PENDING',
                'RUNNING',
                'COMPLETED',
                'FAILED',
                'CANCELLED'
            )
        ),

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_av_scan_agent
        FOREIGN KEY (agent_id)
        REFERENCES agents(id)
        ON DELETE CASCADE,

    CONSTRAINT fk_av_scan_command
        FOREIGN KEY (command_id)
        REFERENCES commands(id)
        ON DELETE CASCADE
);
CREATE INDEX idx_users_role_id
ON users(role_id);

CREATE INDEX idx_users_status
ON users(status);


CREATE INDEX idx_user_sessions_user_id
ON user_sessions(user_id);

CREATE INDEX idx_user_sessions_expires_at
ON user_sessions(expires_at);


CREATE INDEX idx_role_permissions_permission_id
ON role_permissions(permission_id);


CREATE INDEX idx_agents_room_id
ON agents(room_id);

CREATE INDEX idx_agents_status
ON agents(status);

CREATE INDEX idx_agents_last_seen
ON agents(last_seen);


CREATE INDEX idx_commands_agent_id
ON commands(agent_id);

CREATE INDEX idx_commands_issued_by
ON commands(issued_by);

CREATE INDEX idx_commands_status
ON commands(status);

CREATE INDEX idx_commands_created_at
ON commands(created_at DESC);


CREATE INDEX idx_logs_user_id
ON logs(user_id);

CREATE INDEX idx_logs_agent_id
ON logs(target_agent_id);

CREATE INDEX idx_logs_created_at
ON logs(created_at DESC);


CREATE INDEX idx_files_uploaded_by
ON files(uploaded_by);


CREATE INDEX idx_av_scan_results_agent_id
ON av_scan_results(agent_id);

CREATE INDEX idx_av_scan_results_command_id
ON av_scan_results(command_id);


-- =========================
--  Add tokens table
-- =========================
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    user_id UUID NULL,
    agent_id UUID NULL,

    token TEXT NOT NULL,
    token_type VARCHAR(50) NOT NULL,

    max_use INT DEFAULT NULL,
    used_count INT NOT NULL DEFAULT 0,

    expires_at TIMESTAMPTZ,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_tokens_token UNIQUE (token),

    CONSTRAINT fk_tokens_user
        FOREIGN KEY (user_id)
        REFERENCES users(id)
        ON DELETE SET NULL,

    CONSTRAINT fk_tokens_agent
        FOREIGN KEY (agent_id)
        REFERENCES agents(id)
        ON DELETE SET NULL
);

-- Indexes for tokens
CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_tokens_agent_id ON tokens (agent_id);
-- Unique index on token (constraint creates one, but ensure named index exists)
CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_token_unique ON tokens (token);

-- Optional: partial index for active tokens to speed up validation queries
CREATE INDEX IF NOT EXISTS idx_tokens_active_token ON tokens (token)
WHERE is_revoked = FALSE AND (expires_at IS NULL OR expires_at > NOW());

-- Comments for documentation
COMMENT ON TABLE tokens IS 'เก็บ Token ต่าง ๆ ที่ผูกกับผู้ใช้หรือเครื่องลูก (agent) (เช่น refresh token, api key)';
COMMENT ON COLUMN tokens.id IS 'Primary key (UUID, gen_random_uuid())';
COMMENT ON COLUMN tokens.user_id IS 'อ้างอิงไปยัง users.id; จะเป็น NULL ถ้า token ไม่ได้ผูกกับผู้ใช้';
COMMENT ON COLUMN tokens.agent_id IS 'อ้างอิงไปยัง agents.id; จะเป็น NULL ถ้า token ไม่ได้ผูกกับ agent';
COMMENT ON COLUMN tokens.token IS 'ค่า Token จริง (เก็บเป็น text); ต้องไม่ซ้ำ';
COMMENT ON COLUMN tokens.token_type IS 'ประเภทของ token เช่น ''refresh_token'', ''api_key''';
COMMENT ON COLUMN tokens.max_use IS 'จำนวนครั้งสูงสุดที่ token นี้สามารถใช้งานได้; NULL = ไม่จำกัด';
COMMENT ON COLUMN tokens.used_count IS 'จำนวนครั้งที่ token ถูกใช้งานแล้ว (นับเพิ่มเมื่อใช้)';
COMMENT ON COLUMN tokens.expires_at IS 'เวลาที่ token หมดอายุ (timestamptz)';
COMMENT ON COLUMN tokens.is_revoked IS 'สถานะการยกเลิก token; TRUE = ถูกยกเลิก';
COMMENT ON COLUMN tokens.created_at IS 'เวลาที่สร้าง record (timestamp with time zone)';