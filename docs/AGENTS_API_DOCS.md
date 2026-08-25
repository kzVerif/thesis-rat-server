# เอกสารการใช้งาน Agents API

## ข้อมูลทั่วไป

- Base URL: `http://localhost:8080`
- ทุก endpoint ต้อง Login และส่ง session cookie ชื่อ `__Host-session`
- บัญชีผู้เรียกต้องมีสถานะ `ACTIVE`
- Request body ใช้ `Content-Type: application/json`
- Agent ID และ `room_id` ใช้ UUID

## Permission

| Endpoint | Permission ที่ใช้ได้ |
| --- | --- |
| `GET /api/agents/` | `agents.read` หรือ `agents.manage` |
| `GET /api/agents/:id` | `agents.read` หรือ `agents.manage` |
| `POST /api/agents/` | `agents.manage` |
| `PUT /api/agents/:id` | `agents.edit` หรือ `agents.manage` |
| `DELETE /api/agents/:id` | `agents.delete` หรือ `agents.manage` |

`agents.manage` เป็นสิทธิ์รวม ใช้ดำเนินการได้ทุก endpoint ส่วนสิทธิ์ย่อยเหมาะสำหรับ Role ที่ต้องการจำกัดหน้าที่เฉพาะด้าน

## โครงสร้างข้อมูล Agent

| ฟิลด์ | ชนิด | รายละเอียด |
| --- | --- | --- |
| `id` | UUID | รหัส Agent สร้างโดยระบบ |
| `room_id` | UUID หรือ null | ห้องที่ Agent สังกัด ต้องเป็นห้องที่มีอยู่จริง |
| `hostname` | string | บังคับ ไม่เกิน 255 ตัวอักษร |
| `os_info` | JSON หรือ null | ข้อมูลระบบปฏิบัติการแบบ JSON |
| `mac_address` | string หรือ null | รูปแบบ `00:11:22:33:44:55` |
| `ip_address` | string หรือ null | IPv4 หรือ IPv6 |
| `status` | string | `ONLINE`, `OFFLINE`, `WARNING` หรือ `DISABLED`; ค่าเริ่มต้นจาก API คือ `OFFLINE` |
| `last_seen` | datetime หรือ null | เวลาที่พบ Agent ล่าสุด รูปแบบ RFC 3339 |
| `enrolled_at` | datetime หรือ null | เวลาที่ Agent ลงทะเบียน รูปแบบ RFC 3339 |
| `created_at` | datetime | เวลาที่สร้างข้อมูล |
| `updated_at` | datetime | เวลาที่แก้ไขล่าสุด |

## การ Login

```bash
curl -i -c cookies.txt \
  -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

คำสั่งตัวอย่างถัดไปจะอ่าน cookie จาก `cookies.txt`

## การกำหนด Permission ให้ Role

Permission ทั้งสี่รายการอยู่ใน `schema.sql` และ `seed.sql` แล้ว สำหรับฐานข้อมูลเดิมให้รันคำสั่ง SQL ต่อไปนี้ หรือสร้างผ่าน Permissions API:

```sql
INSERT INTO permissions (code, description)
VALUES
  ('agents.manage', 'จัดการเครื่องลูกทั้งหมด'),
  ('agents.read', 'ดูรายชื่อและข้อมูลเครื่องลูก'),
  ('agents.edit', 'แก้ไขข้อมูลเครื่องลูก'),
  ('agents.delete', 'ลบเครื่องลูก')
ON CONFLICT (code) DO NOTHING;
```

ดู UUID ของ Permission:

```bash
curl -i -b cookies.txt http://localhost:8080/api/permissions/
```

จากนั้นนำ UUID ไปใส่ใน `permission_ids` ตอนแก้ไข Role:

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/roles/ROLE_UUID \
  -H "Content-Type: application/json" \
  -d '{
    "name":"AGENT_OPERATOR",
    "description":"ดูและแก้ไข Agent",
    "permission_ids":["AGENTS_READ_UUID","AGENTS_EDIT_UUID"]
  }'
```

> การส่ง `permission_ids` จะแทนที่ Permission เดิมทั้งหมดของ Role ต้องส่ง UUID ของ Permission เดิมที่ต้องการเก็บไว้รวมมาด้วย

## 1. ดูรายการ Agent

```http
GET /api/agents/?page=1&limit=20
```

```bash
curl -i -b cookies.txt "http://localhost:8080/api/agents/?page=1&limit=20"
```

Query parameters:

| Parameter | ค่าเริ่มต้น | รายละเอียด |
| --- | --- | --- |
| `page` | `1` | หน้าที่ต้องการ ต้องเป็นจำนวนเต็มมากกว่า 0 |
| `limit` | `20` | จำนวนรายการต่อหน้า ต้องอยู่ระหว่าง 1 ถึง 100 |

Response: `200 OK`

```json
{
  "agents": [
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "room_id": "22222222-2222-2222-2222-222222222222",
      "hostname": "LAB-PC-01",
      "os_info": {"name":"Windows","version":"11"},
      "mac_address": "00:11:22:33:44:55",
      "ip_address": "192.168.1.10",
      "status": "ONLINE",
      "last_seen": "2026-08-16T10:30:00+07:00",
      "enrolled_at": "2026-08-01T09:00:00+07:00",
      "created_at": "2026-08-01T09:00:00+07:00",
      "updated_at": "2026-08-16T10:30:00+07:00"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 45,
    "total_pages": 3
  }
}
```

รายการเรียงตาม `hostname` และ `id` เพื่อให้ลำดับคงที่ หากไม่มีข้อมูล `agents` จะเป็น `[]`, `total` และ `total_pages` จะเป็น `0` หากขอหน้าที่เกินหน้าสุดท้ายจะตอบ `200 OK` พร้อม `agents: []`

## 2. ดู Agent ตาม ID

```http
GET /api/agents/:id
```

```bash
curl -i -b cookies.txt \
  http://localhost:8080/api/agents/11111111-1111-1111-1111-111111111111
```

สำเร็จจะตอบ `200 OK` พร้อมข้อมูล Agent หนึ่งรายการ หากไม่พบจะตอบ `404 Not Found`

## 3. สร้าง Agent

```http
POST /api/agents/
Content-Type: application/json
```

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/agents/ \
  -H "Content-Type: application/json" \
  -d '{
    "room_id":"22222222-2222-2222-2222-222222222222",
    "hostname":"LAB-PC-01",
    "os_info":{"name":"Windows","version":"11"},
    "mac_address":"00:11:22:33:44:55",
    "ip_address":"192.168.1.10",
    "status":"OFFLINE",
    "last_seen":null,
    "enrolled_at":"2026-08-16T09:00:00+07:00"
  }'
```

ส่งเฉพาะ `hostname` ได้ ฟิลด์อื่นเป็น optional และ `status` จะเป็น `OFFLINE` หากไม่ส่ง

Response: `201 Created`

```json
{
  "message": "agent created",
  "id": "11111111-1111-1111-1111-111111111111"
}
```

## 4. แก้ไข Agent

```http
PUT /api/agents/:id
Content-Type: application/json
```

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/agents/11111111-1111-1111-1111-111111111111 \
  -H "Content-Type: application/json" \
  -d '{
    "room_id":null,
    "hostname":"LAB-PC-01-RENAMED",
    "os_info":{"name":"Windows","version":"11","build":"24H2"},
    "mac_address":"00:11:22:33:44:55",
    "ip_address":"192.168.1.20",
    "status":"WARNING",
    "last_seen":"2026-08-16T11:00:00+07:00",
    "enrolled_at":"2026-08-16T09:00:00+07:00"
  }'
```

Response: `200 OK`

```json
{"message":"agent updated"}
```

> `PUT` แทนค่าข้อมูลทั้งรายการ ต้องส่ง `hostname` ทุกครั้ง ฟิลด์ optional ที่ไม่ส่งจะถูกเปลี่ยนเป็น `null` และ `status` ที่ไม่ส่งจะเป็น `OFFLINE`

## 5. ลบ Agent

```http
DELETE /api/agents/:id
```

```bash
curl -i -b cookies.txt \
  -X DELETE http://localhost:8080/api/agents/11111111-1111-1111-1111-111111111111
```

สำเร็จจะตอบ `204 No Content` โดยไม่มี response body

> การลบ Agent จะลบ `commands` ของ Agent นั้นตาม Foreign Key `ON DELETE CASCADE` ด้วย จึงควรให้ `agents.delete` เฉพาะ Role ที่เชื่อถือได้

## Error responses

| HTTP Status | กรณี |
| --- | --- |
| `400 Bad Request` | JSON, UUID, hostname, status, MAC, IP, datetime, `page` หรือ `limit` ไม่ถูกต้อง หรือไม่พบ `room_id` |
| `401 Unauthorized` | ไม่ได้ Login, session หมดอายุ หรือบัญชีไม่ใช่ `ACTIVE` |
| `403 Forbidden` | Role ไม่มี Permission ที่ endpoint ต้องการ |
| `404 Not Found` | ไม่พบ Agent ตาม ID |
| `500 Internal Server Error` | ฐานข้อมูลหรือการตรวจสอบ Permission มีปัญหา |

ตัวอย่าง:

```json
{"error":"ไม่มีสิทธิ์ดำเนินการนี้"}
```

## สรุป endpoint

| Method | Path | ผลลัพธ์สำเร็จ |
| --- | --- | --- |
| `GET` | `/api/agents/` | `200 OK` |
| `GET` | `/api/agents/:id` | `200 OK` |
| `POST` | `/api/agents/` | `201 Created` |
| `PUT` | `/api/agents/:id` | `200 OK` |
| `DELETE` | `/api/agents/:id` | `204 No Content` |
