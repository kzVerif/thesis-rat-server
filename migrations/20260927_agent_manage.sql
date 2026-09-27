BEGIN;

INSERT INTO permissions(code, description)
VALUES('rooms.read', 'ดูรายการและรายละเอียดห้อง')
ON CONFLICT (code) DO NOTHING;

-- Consolidate every legacy CRUD grant into the new full CRUD permission.
INSERT INTO role_permissions(role_id, permission_id)
SELECT DISTINCT rp.role_id, target.id
FROM role_permissions rp
JOIN permissions old ON old.id=rp.permission_id
CROSS JOIN permissions target
WHERE old.code IN ('agents.manage', 'agents.read', 'agents.edit', 'agents.delete')
  AND target.code='agent.manage'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT rp.role_id, target.id
FROM role_permissions rp
JOIN permissions source ON source.id=rp.permission_id
CROSS JOIN permissions target
WHERE source.code='agent.manage' AND target.code='rooms.read'
ON CONFLICT DO NOTHING;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE code IN ('agents.manage', 'agents.read', 'agents.edit', 'agents.delete')
);
DELETE FROM permissions
WHERE code IN ('agents.manage', 'agents.read', 'agents.edit', 'agents.delete');

ALTER TABLE av_scan_results DROP COLUMN total_files_scanned;
ALTER TABLE av_scan_results DROP COLUMN threats_found;
COMMIT;
