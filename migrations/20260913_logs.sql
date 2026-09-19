BEGIN;
INSERT INTO permissions(code,description) VALUES ('logs.read','Read audit logs')
ON CONFLICT (code) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.name='ADMINISTRATOR' AND p.code='logs.read'
ON CONFLICT DO NOTHING;
CREATE INDEX IF NOT EXISTS idx_logs_action_created_at ON logs(action,created_at DESC,id DESC);
COMMIT;
