# เอกสารการใช้งาน Users API

## ข้อมูลทั่วไป

- Base URL: `http://localhost:8080`
- Request body ใช้ `Content-Type: application/json`
- ทุก endpoint ต้องเข้าสู่ระบบด้วย session cookie ชื่อ `__Host-session`
- ผู้เรียกต้องมีสถานะผู้ใช้เป็น `ACTIVE`
- Role ของผู้เรียกต้องมี Permission รหัส `users.manage`
- ID ของผู้ใช้และ Role ต้องเป็น UUID
- API จะไม่ส่งรหัสผ่านหรือ `password_hash` กลับมาใน response

Users CRUD API แตกต่างจาก `POST /api/auth/register` โดย Users API ใช้สำหรับผู้มีสิทธิ์จัดการบัญชีผู้ใช้ ส่วน Register ใช้สำหรับสมัครบัญชีใหม่ด้วย Role เริ่มต้น `VIEWER` และสถานะเริ่มต้น `DISABLED` ผู้มีสิทธิ์ `users.manage` ต้องเปลี่ยนสถานะเป็น `ACTIVE` ก่อน บัญชีจึงจะ Login ได้

## โครงสร้างข้อมูลผู้ใช้

| ฟิลด์ | ชนิดข้อมูล | รายละเอียด |
| --- | --- | --- |
| `id` | UUID | รหัสประจำผู้ใช้ |
| `username` | string | ชื่อผู้ใช้ ต้องไม่ว่าง ไม่เกิน 50 ตัวอักษร และห้ามซ้ำ |
| `email` | string หรือ null | อีเมล ไม่เกิน 255 ตัวอักษรและห้ามซ้ำ |
| `display_name` | string หรือ null | ชื่อที่แสดง ไม่เกิน 100 ตัวอักษร |
| `role_id` | UUID | ID ของ Role ที่มีอยู่ในระบบ |
| `role` | string | ชื่อ Role สำหรับแสดงผล |
| `status` | string | `ACTIVE`, `DISABLED` หรือ `LOCKED` |
| `failed_login_attempts` | integer | จำนวนครั้งที่เข้าสู่ระบบไม่สำเร็จ |
| `locked_until` | datetime หรือ null | เวลาสิ้นสุดการล็อกบัญชี |
| `last_login_at` | datetime หรือ null | เวลาที่เข้าสู่ระบบล่าสุด |
| `password_changed_at` | datetime หรือ null | เวลาที่เปลี่ยนรหัสผ่านล่าสุด |
| `created_at` | datetime | เวลาที่สร้างผู้ใช้ |
| `updated_at` | datetime | เวลาที่แก้ไขผู้ใช้ล่าสุด |

## การเข้าสู่ระบบ

```http
POST /api/auth/login
Content-Type: application/json
```

```json
{
  "username": "admin",
  "password": "your-password"
}
```

ตัวอย่าง Login และบันทึก cookie:

```bash
curl -i -c cookies.txt \
  -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

คำสั่ง Users API ด้านล่างจะอ่าน cookie จากไฟล์ `cookies.txt`

> Session จะใช้งานได้เฉพาะขณะที่บัญชีผู้เรียกมีสถานะ `ACTIVE` หากบัญชีถูกเปลี่ยนเป็น `DISABLED` หรือ `LOCKED` request ถัดไปจะไม่ผ่านการยืนยันตัวตน

## การกำหนด Permission ให้ Role

Permission `users.manage` ถูกเพิ่มใน `schema.sql` สำหรับฐานข้อมูลใหม่แล้ว สำหรับฐานข้อมูลเดิมสามารถสร้างผ่าน Permission API:

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/permissions/ \
  -H "Content-Type: application/json" \
  -d '{"code":"users.manage","description":"จัดการข้อมูลผู้ใช้"}'
```

จากนั้นนำ ID ของ Permission ไปเพิ่มใน `permission_ids` ของ Role:

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/roles/ROLE_UUID \
  -H "Content-Type: application/json" \
  -d '{
    "name":"USER_MANAGER",
    "description":"ผู้จัดการบัญชีผู้ใช้",
    "permission_ids":["USERS_MANAGE_PERMISSION_UUID"]
  }'
```

> Role Update จะแทนที่ Permission เดิมทั้งหมดเมื่อส่ง `permission_ids` จึงต้องส่ง Permission ID เดิมที่ต้องการเก็บไว้รวมมาด้วย

## 1. ดูรายการผู้ใช้

```http
GET /api/users/
```

```bash
curl -i -b cookies.txt http://localhost:8080/api/users/
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "users": [
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "username": "operator01",
      "email": "operator01@example.com",
      "display_name": "Operator 01",
      "role_id": "22222222-2222-2222-2222-222222222222",
      "role": "VIEWER",
      "status": "ACTIVE",
      "failed_login_attempts": 0,
      "locked_until": null,
      "last_login_at": "2026-08-11T10:00:00+07:00",
      "password_changed_at": null,
      "created_at": "2026-08-11T09:00:00+07:00",
      "updated_at": "2026-08-11T09:00:00+07:00"
    }
  ]
}
```

หากไม่มีผู้ใช้ ระบบจะตอบ `"users": []`

## 2. ดูผู้ใช้ตาม ID

```http
GET /api/users/:id
```

```bash
curl -i -b cookies.txt \
  http://localhost:8080/api/users/11111111-1111-1111-1111-111111111111
```

Response สำเร็จ: HTTP `200 OK` โดยใช้โครงสร้างข้อมูลผู้ใช้แบบเดียวกับรายการด้านบน แต่ไม่ครอบด้วยฟิลด์ `users`

```json
{
  "id": "11111111-1111-1111-1111-111111111111",
  "username": "operator01",
  "email": "operator01@example.com",
  "display_name": "Operator 01",
  "role_id": "22222222-2222-2222-2222-222222222222",
  "role": "VIEWER",
  "status": "ACTIVE",
  "failed_login_attempts": 0,
  "locked_until": null,
  "last_login_at": null,
  "password_changed_at": null,
  "created_at": "2026-08-11T09:00:00+07:00",
  "updated_at": "2026-08-11T09:00:00+07:00"
}
```

## 3. สร้างผู้ใช้

```http
POST /api/users/
Content-Type: application/json
```

Request body:

```json
{
  "username": "operator01",
  "email": "operator01@example.com",
  "password": "strong-password",
  "display_name": "Operator 01",
  "role_id": "22222222-2222-2222-2222-222222222222",
  "status": "ACTIVE"
}
```

ข้อกำหนด:

- `username`, `password`, `role_id` และ `status` จำเป็นต้องส่ง
- รหัสผ่านต้องมีอย่างน้อย 8 ตัวอักษร
- `email` และ `display_name` สามารถส่งเป็น `null` หรือไม่ส่งก็ได้
- `role_id` ต้องอ้างอิง Role ที่มีอยู่จริง
- `status` รองรับ `ACTIVE`, `DISABLED` และ `LOCKED` โดยไม่คำนึงถึงตัวพิมพ์เล็กหรือใหญ่

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/users/ \
  -H "Content-Type: application/json" \
  -d '{
    "username":"operator01",
    "email":"operator01@example.com",
    "password":"strong-password",
    "display_name":"Operator 01",
    "role_id":"22222222-2222-2222-2222-222222222222",
    "status":"ACTIVE"
  }'
```

Response สำเร็จ: HTTP `201 Created`

```json
{
  "message": "สร้างผู้ใช้สำเร็จ",
  "id": "11111111-1111-1111-1111-111111111111"
}
```

## 4. แก้ไขผู้ใช้

```http
PUT /api/users/:id
Content-Type: application/json
```

Request body:

```json
{
  "username": "operator01",
  "email": "new-email@example.com",
  "display_name": "Senior Operator",
  "role_id": "22222222-2222-2222-2222-222222222222",
  "status": "ACTIVE"
}
```

ข้อกำหนด:

- PUT เป็นการแทนค่าข้อมูล จึงต้องส่ง `username`, `role_id` และ `status` ทุกครั้ง
- หากไม่ส่ง `email` หรือ `display_name` ค่าของฟิลด์นั้นจะเปลี่ยนเป็น `null`
- หากไม่ส่ง `password` หรือส่งเป็นค่าว่าง ระบบจะเก็บรหัสผ่านเดิมไว้
- หากส่ง `password` ต้องมีอย่างน้อย 8 ตัวอักษร และ `password_changed_at` จะถูกอัปเดต
- การเปลี่ยน Role หรือ Status ของบัญชีที่กำลัง Login จะมีผลกับ request ถัดไป

ตัวอย่างเปลี่ยนข้อมูลโดยเก็บรหัสผ่านเดิม:

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/users/11111111-1111-1111-1111-111111111111 \
  -H "Content-Type: application/json" \
  -d '{
    "username":"operator01",
    "email":"new-email@example.com",
    "display_name":"Senior Operator",
    "role_id":"22222222-2222-2222-2222-222222222222",
    "status":"ACTIVE"
  }'
```

ตัวอย่างเปลี่ยนข้อมูลพร้อมรหัสผ่าน:

```json
{
  "username": "operator01",
  "email": "new-email@example.com",
  "password": "new-strong-password",
  "display_name": "Senior Operator",
  "role_id": "22222222-2222-2222-2222-222222222222",
  "status": "ACTIVE"
}
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "แก้ไขผู้ใช้สำเร็จ"
}
```

## 5. ลบผู้ใช้

```http
DELETE /api/users/:id
```

```bash
curl -i -b cookies.txt \
  -X DELETE http://localhost:8080/api/users/11111111-1111-1111-1111-111111111111
```

Response สำเร็จ: HTTP `204 No Content` และไม่มี response body

Session ของผู้ใช้ที่ถูกลบจะถูกลบอัตโนมัติด้วย `ON DELETE CASCADE` หากผู้ใช้ถูกอ้างอิงโดยข้อมูลที่ป้องกันการลบ เช่น Commands หรือ Files ระบบจะตอบ HTTP `409 Conflict`

> API อนุญาตให้ผู้ใช้ลบบัญชีของตนเองได้ หากมี Permission `users.manage` เมื่อสำเร็จ session ปัจจุบันจะใช้งานไม่ได้อีก

## รหัสข้อผิดพลาด

| HTTP Status | กรณี | ตัวอย่าง Response |
| --- | --- | --- |
| `400 Bad Request` | UUID, request body, Role, Status หรือข้อมูลไม่ถูกต้อง | `{"error":"รูปแบบข้อมูลไม่ถูกต้อง"}` |
| `401 Unauthorized` | ไม่ได้ Login, session หมดอายุ หรือบัญชีผู้เรียกไม่ได้เป็น `ACTIVE` แล้ว | `{"error":"ไม่ได้รับอนุญาตให้เข้าใช้งาน"}` |
| `403 Forbidden` | Role ของผู้เรียกไม่มี Permission `users.manage` | `{"error":"ไม่มีสิทธิ์ดำเนินการนี้"}` |
| `404 Not Found` | ไม่พบผู้ใช้ตาม ID | `{"error":"ไม่พบผู้ใช้"}` |
| `409 Conflict` | Username/Email ซ้ำ หรือผู้ใช้ถูกข้อมูลอื่นอ้างอิงและลบไม่ได้ | `{"error":"ชื่อผู้ใช้หรืออีเมลนี้ถูกใช้งานแล้ว"}` |
| `500 Internal Server Error` | ไม่สามารถอ่านหรือดำเนินการกับข้อมูลผู้ใช้ได้ | `{"error":"ไม่สามารถดำเนินการกับข้อมูลผู้ใช้ได้"}` |

## ลำดับการตรวจสอบสิทธิ์

ทุก request ของ Users API จะถูกตรวจตามลำดับนี้:

1. ตรวจ session cookie ว่ายังใช้งานได้
2. ตรวจว่าบัญชีผู้เรียกมีสถานะ `ACTIVE`
3. อ่าน Role ปัจจุบันของผู้เรียก
4. ตรวจว่า Role เชื่อมกับ Permission `users.manage`
5. เรียก Users handler เมื่อผ่านทุกเงื่อนไข

การเพิ่มหรือถอน `users.manage` จาก Role มีผลกับ request ครั้งถัดไปทันที โดยไม่จำเป็นต้อง Login ใหม่
