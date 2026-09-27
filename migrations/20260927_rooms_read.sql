BEGIN;

INSERT INTO permissions(code, description)
VALUES ('rooms.read', 'ดูรายการและรายละเอียดห้อง')
ON CONFLICT (code) DO NOTHING;

-- Assign rooms.read to other roles explicitly through role management.
-- Existing rooms.manage grants continue to allow both reads and writes.

COMMIT;
