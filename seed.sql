-- =========================
-- 1) สร้าง Roles
-- =========================

WITH inserted_role AS (
    INSERT INTO roles (name, description)
    VALUES (
        'ADMINISTRATOR',
        'ผู้ดูแลระบบทั้งหมด'
    )
    ON CONFLICT (name) DO UPDATE
    SET description = EXCLUDED.description
    RETURNING id
)
INSERT INTO users (
    username,
    email,
    password_hash,
    display_name,
    role_id,
    status
)
SELECT
    'admin',
    'admin@example.com',
    '$2a$10$q2Zjt4nMlq8tY1trMefwnOpohEwAVWjT7dTafZjuCQaynD5ZlF5u.',
    'Administrator',
    id,
    'ACTIVE'
FROM inserted_role
ON CONFLICT (username) DO NOTHING;


INSERT INTO roles (name, description)
VALUES (
    'VIEWER',
    'ผู้ชม'
)
ON CONFLICT (name) DO UPDATE
SET description = EXCLUDED.description;


-- =========================
-- 2) สร้าง Permissions
-- =========================

INSERT INTO permissions (code, description)
VALUES
    ('role.manage', 'จัดการบทบาทและสิทธิ์ของบทบาท'),
    ('av.read', 'อ่านข้อมูลการสแกนไวรัส'),
    ('agents.manage', 'จัดการเครื่องลูกทั้งหมด'),
    ('rooms.manage', 'จัดการห้อง'),
    ('tokens.manage', 'จัดการ Token'),
    ('agents.read', 'ดูรายชื่อเครื่องลูก'),
    ('agents.control', 'ควบคุมเครื่องลูก'),
    ('agents.delete', 'ลบเครื่องลูก'),

    ('files.distribute', 'กระจายไฟล์'),
    ('files.upload', 'อัปโหลดไฟล์สู่เซิร์ฟเวอร์'),
    ('files.manage', 'จัดการไฟล์ภายในเซิร์ฟเวอร์'),
    ('files.delete', 'ลบไฟล์บนเซิร์ฟเวอร์'),

    ('users.manage', 'จัดการผู้ใช้ในระบบ'),
    ('logs.read', 'อ่านข้อมูล Log ภายในระบบ'),
    ('agents.edit', 'แก้ไขข้อมูลเครื่องลูก'),

    ('monitor.read', 'ดูหน้าจอเครื่องลูก')
ON CONFLICT (code) DO UPDATE
SET description = EXCLUDED.description;


-- =========================
-- 3) ADMINISTRATOR
-- ให้ทุก Permission
-- =========================

INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'ADMINISTRATOR'
ON CONFLICT (role_id, permission_id) DO NOTHING;


-- =========================
-- 4) VIEWER
-- ให้เฉพาะ monitor.read
-- =========================

INSERT INTO role_permissions (
    role_id,
    permission_id
)
SELECT
    r.id,
    p.id
FROM roles r
JOIN permissions p
    ON p.code = 'monitor.read'
WHERE r.name = 'VIEWER'
ON CONFLICT (role_id, permission_id) DO NOTHING;
