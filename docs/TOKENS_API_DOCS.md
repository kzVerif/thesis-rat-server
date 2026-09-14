# API สำหรับ Enrollment Token ของ Agent

Token ใช้สำหรับลงทะเบียน Agent ครั้งแรกเท่านั้น ไม่ใช่ session token และระบบจะไม่บันทึกว่า Agent ใดใช้ token ใดลงทะเบียน

เส้นจัดการ token ต้องเข้าสู่ระบบด้วย session ที่ยังใช้งานได้ และต้องมีสิทธิ์ `tokens.manage` ส่วนเส้นตรวจ token และลงทะเบียน Agent ไม่ต้องใช้ session ของผู้ใช้

## รายการ API

| Method | Path | การยืนยันตัวตน | หน้าที่ |
|---|---|---|---|
| GET | `/api/tokens` | session + `tokens.manage` | อ่านรายการ token |
| GET | `/api/tokens/:id` | session + `tokens.manage` | อ่าน token รายการเดียว |
| POST | `/api/tokens` | session + `tokens.manage` | สร้าง enrollment token |
| PUT/PATCH | `/api/tokens/:id` | session + `tokens.manage` | แก้ไขเฉพาะ `max_use` และ `expires_at` |
| DELETE | `/api/tokens/:id` | session + `tokens.manage` | ยุติการใช้งาน token โดยเก็บประวัติไว้ |
| POST | `/api/tokens/validate` | ไม่ต้องใช้ session | ตรวจสอบ token โดยไม่ใช้โควตา |
| POST | `/api/agents/register` | ไม่ต้องใช้ session | ลงทะเบียน Agent และใช้โควตา 1 ครั้ง |

ไม่มี session จะได้ `401` และไม่มีสิทธิ์จะได้ `403`

## โครงสร้างข้อมูลในตาราง tokens

ตาราง `tokens` มีข้อมูลดังนี้

- `id`: UUID ของ token
- `created_by`: UUID ผู้ใช้ที่สร้าง token ถ้าผู้ใช้ถูกลบจะเป็น `NULL`
- `token_hash`: ค่า SHA-256 ของ token จริง ระบบไม่เก็บ token แบบ plaintext
- `max_use`: จำนวนครั้งสูงสุดที่ใช้ลงทะเบียน Agent ได้ ถ้าเป็น `NULL` คือไม่จำกัด
- `used_count`: จำนวนครั้งที่ลงทะเบียน Agent สำเร็จแล้ว
- `expires_at`: วันเวลาหมดอายุ ถ้าเป็น `NULL` คือไม่กำหนดวันหมดอายุ
- `is_revoked`: สถานะยุติการใช้งาน
- `created_at`: วันที่สร้าง
- `updated_at`: วันที่แก้ไขล่าสุด

ไม่มีคอลัมน์ `agent_id`, `user_id` หรือ `token_type` เพราะระบบไม่ต้องการเก็บความสัมพันธ์ว่า Agent ใดสมัครด้วย token ใด

## สร้าง Token

```http
POST /api/tokens
Content-Type: application/json
Cookie: session=...
```

```json
{
  "max_use": 10,
  "expires_at": "2026-12-31T23:59:59+07:00"
}
```

ทั้งสองฟิลด์เป็น optional

- `max_use` ต้องเป็นจำนวนเต็มตั้งแต่ 1 ถึง 2,147,483,647 หรือ `null`
- `expires_at` ต้องเป็นเวลา RFC3339 ในอนาคต หรือ `null`

Response `201` จะคืน token plaintext ให้เฉพาะตอนสร้างเท่านั้น พร้อมข้อมูล metadata ของ token และตั้งค่า `Cache-Control: no-store`

ตัวอย่าง response:

```json
{
  "id": "uuid",
  "created_by": "user-uuid",
  "creator_username": "admin",
  "max_use": 10,
  "used_count": 0,
  "expires_at": "2026-12-31T16:59:59Z",
  "is_revoked": false,
  "created_at": "2026-09-14T10:00:00Z",
  "updated_at": "2026-09-14T10:00:00Z",
  "token": "plaintext-secret"
}
```

ควรเก็บค่า `token` ไว้อย่างปลอดภัย เพราะ API รายการและ API รายการเดียวจะไม่คืนค่า token หรือ hash อีก

## อ่านรายการ Token

```http
GET /api/tokens?page=1&limit=20
```

รายการเรียงจาก token ที่สร้างล่าสุดไปเก่าสุด โดย `limit` ต้องอยู่ระหว่าง 1 ถึง 100

```json
{
  "tokens": [],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 0,
    "total_pages": 0
  }
}
```

`GET /api/tokens/:id` ใช้อ่านข้อมูล token รายการเดียว และไม่คืนค่า plaintext หรือ hash ของ token

- UUID ไม่ถูกต้อง: `400`
- ไม่พบ token: `404`

## แก้ไข Token

```http
PATCH /api/tokens/:id
Content-Type: application/json
```

```json
{
  "max_use": 25,
  "expires_at": "2027-01-31T23:59:59Z"
}
```

แก้ไขได้เฉพาะ `max_use` และ `expires_at` เท่านั้น ฟิลด์อื่น เช่น `is_revoked`, `token`, `created_by`, `used_count`, `agent_id` หรือ `user_id` จะถูกปฏิเสธด้วย `400`

ข้อกำหนดเพิ่มเติม:

- `max_use` ใหม่ต้องไม่น้อยกว่า `used_count` ปัจจุบัน ถ้าน้อยกว่าจะได้ `409`
- ส่ง `null` เพื่อล้างค่า `max_use` หรือ `expires_at`
- `PUT` และ `PATCH` มีพฤติกรรมเหมือนกัน
- ไม่ส่งฟิลด์ที่แก้ไขได้เลยจะได้ `400`

Response สำเร็จ:

```json
{ "message": "updated" }
```

## ยุติการใช้งาน Token

```http
DELETE /api/tokens/:id
```

คำสั่งนี้จะตั้งค่า `is_revoked=true` ไม่ได้ลบแถวออกจากฐานข้อมูล จึงยังเก็บข้อมูลผู้สร้าง จำนวนครั้งที่ใช้ และประวัติวันเวลาไว้สำหรับตรวจสอบภายหลัง

การ revoke ซ้ำจะยังได้ `200` ส่วน token ที่ไม่มีอยู่จะได้ `404`

```json
{ "message": "revoked" }
```

## ตรวจสอบ Token โดยไม่ใช้โควตา

```http
POST /api/tokens/validate
Content-Type: application/json

{ "token": "plaintext-secret" }
```

API นี้ตรวจสอบ hash, สถานะ revoke, วันหมดอายุ และโควตาที่เหลือ แต่จะไม่เพิ่ม `used_count`

กรณีสำเร็จ:

```json
{ "valid": true }
```

Token ที่ไม่ถูกต้อง หมดอายุ ถูก revoke หรือใช้ครบแล้วจะได้ `403` พร้อม `valid:false` ค่า token จะไม่ถูกเขียนลง log

## ลงทะเบียน Agent ครั้งแรก

```http
POST /api/agents/register
Content-Type: application/json

{
  "token": "plaintext-secret",
  "hostname": "workstation-01",
  "mac_address": "AA:BB:CC:DD:EE:FF",
  "os_info": { "name": "Windows" },
  "room_id": "room-uuid"
}
```

ฟิลด์ที่จำเป็น:

- `token`
- `hostname`
- `mac_address`

ฟิลด์ที่ไม่บังคับ:

- `os_info`
- `room_id`

เมื่อสำเร็จ ระบบจะสร้าง Agent ด้วยสถานะ `OFFLINE` และกำหนด `enrolled_at`

การเพิ่ม `used_count` และการสร้าง Agent ทำใน transaction เดียวกัน ดังนั้น:

- สมัครสำเร็จ: เพิ่ม `used_count` 1 ครั้ง
- สร้าง Agent ไม่สำเร็จ: quota จะถูก rollback และไม่นับการใช้
- สมัครพร้อมกันหลายเครื่อง: ระบบล็อกการใช้ token ทำให้ไม่ใช้เกิน `max_use`
- MAC address ซ้ำ: ได้ `409`
- ข้อมูล Agent ไม่ถูกต้อง: ได้ `400`
- token ไม่ถูกต้อง หมดอายุ ถูก revoke หรือใช้ครบ: ได้ `403`

Response สำเร็จ:

```json
{
  "id": "agent-uuid",
  "message": "agent registered"
}
```

ข้อมูล Agent จะไม่มี token ID และไม่มีค่า token ที่ใช้สมัคร

## การ Migration ฐานข้อมูลเดิม

สำหรับฐานข้อมูลเดิม ให้หยุด server ก่อน แล้วรัน [20260913_agent_enrollment_tokens.sql](../migrations/20260913_agent_enrollment_tokens.sql)

Migration นี้จะ:

- hash token plaintext เดิม
- revoke token เดิมทั้งหมด
- ลบคอลัมน์เก่าที่ผูก token กับ user หรือ agent
- เก็บประวัติ `used_count` และวันเวลาที่มีอยู่เดิม
- เพิ่ม `tokens.manage`
- มอบสิทธิ์ `tokens.manage` ให้ Role `ADMINISTRATOR`
- เพิ่ม unique index สำหรับ MAC address ของ Agent

ไม่สามารถกู้คืน token plaintext เดิมได้ หลัง migration ต้องสร้าง token ใหม่ หากมี Agent เดิมที่ใช้ MAC ซ้ำกัน ต้องแก้ไขข้อมูลก่อน เพราะ migration จะ rollback และไม่ลบ Agent อัตโนมัติ

ฐานข้อมูลใหม่ให้ใช้ `schema.sql` เวอร์ชันล่าสุด ส่วน Role อื่นนอกจาก `ADMINISTRATOR` ต้องได้รับสิทธิ์ `tokens.manage` ผ่าน Role management API
