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
    ('users.manage', 'จัดการข้อมูลผู้ใช้'),
    ('agents.manage', 'จัดการเครื่องลูกทั้งหมด'),
    ('agents.read', 'ดูรายชื่อและข้อมูลเครื่องลูก'),
    ('agents.edit', 'แก้ไขข้อมูลเครื่องลูก'),
    ('agents.delete', 'ลบเครื่องลูก')
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
