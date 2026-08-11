# เอกสารการใช้งาน Rooms API

## ข้อมูลทั่วไป

- Base URL: `http://localhost:8080`
- Request body ใช้ `Content-Type: application/json`
- ทุก endpoint ต้องเข้าสู่ระบบและส่ง session cookie ชื่อ `__Host-session`
- Role ของผู้ใช้ต้องมี Permission รหัส `rooms.manage`
- ID ของห้องต้องเป็น UUID

ข้อมูลห้องประกอบด้วย:

| ฟิลด์ | ชนิดข้อมูล | รายละเอียด |
| --- | --- | --- |
| `id` | UUID | รหัสประจำห้อง |
| `name` | string | ชื่อห้อง ต้องไม่ว่าง ไม่เกิน 100 ตัวอักษร และห้ามซ้ำ |
| `description` | string หรือ null | รายละเอียดห้อง |
| `created_at` | datetime | วันเวลาที่สร้าง |
| `updated_at` | datetime | วันเวลาที่แก้ไขล่าสุด |
| `agent_count` | integer | จำนวน Agent ที่ลงทะเบียนและผูกกับห้อง |
| `online_agent_count` | integer | จำนวน Agent ในห้องที่มีสถานะ `ONLINE` |
| `offline_agent_count` | integer | จำนวน Agent ในห้องที่มีสถานะ `OFFLINE` |

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

ตัวอย่างการ Login และบันทึก cookie:

```bash
curl -i -c cookies.txt \
  -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

คำสั่งตัวอย่างของ Rooms API ด้านล่างจะอ่าน cookie จากไฟล์ `cookies.txt`

> Cookie กำหนดเป็น `Secure` สำหรับการใช้งานจริงผ่าน HTTPS หากทดสอบในสภาพแวดล้อม HTTPS ที่ใช้ certificate ภายใน สามารถเพิ่ม `-k` ให้ `curl` ได้

## การกำหนด Permission ให้ Role

Permission `rooms.manage` ถูกเพิ่มไว้ใน `schema.sql` สำหรับฐานข้อมูลใหม่แล้ว สำหรับฐานข้อมูลเดิมสามารถสร้างผ่าน Permission API:

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/permissions/ \
  -H "Content-Type: application/json" \
  -d '{"code":"rooms.manage","description":"จัดการข้อมูลห้อง"}'
```

จากนั้นนำ ID ของ Permission ไปเพิ่มใน `permission_ids` ของ Role:

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/roles/ROLE_UUID \
  -H "Content-Type: application/json" \
  -d '{
    "name":"ROOM_MANAGER",
    "description":"ผู้จัดการห้อง",
    "permission_ids":["ROOMS_MANAGE_PERMISSION_UUID"]
  }'
```

> เมื่อส่ง `permission_ids` ใน Role Update ระบบจะแทนที่รายการ Permission เดิมทั้งหมด จึงต้องส่ง ID เดิมที่ต้องการเก็บไว้รวมกับ `rooms.manage` ด้วย

## 1. ดูรายการห้อง

```http
GET /api/rooms/
```

```bash
curl -i -b cookies.txt http://localhost:8080/api/rooms/
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "rooms": [
    {
      "id": "11111111-1111-1111-1111-111111111111",
      "name": "Computer Lab 1",
      "description": "ห้องปฏิบัติการชั้น 2",
      "created_at": "2026-08-11T10:00:00+07:00",
      "updated_at": "2026-08-11T10:00:00+07:00",
      "agent_count": 12,
      "online_agent_count": 8,
      "offline_agent_count": 3
    }
  ]
}
```

หากไม่มีข้อมูล ระบบจะตอบ `"rooms": []`

## 2. ดูห้องตาม ID

```http
GET /api/rooms/:id
```

```bash
curl -i -b cookies.txt \
  http://localhost:8080/api/rooms/11111111-1111-1111-1111-111111111111
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "id": "11111111-1111-1111-1111-111111111111",
  "name": "Computer Lab 1",
  "description": "ห้องปฏิบัติการชั้น 2",
  "created_at": "2026-08-11T10:00:00+07:00",
  "updated_at": "2026-08-11T10:00:00+07:00",
  "agent_count": 12,
  "online_agent_count": 8,
  "offline_agent_count": 3
}
```

`agent_count` นับ Agent ทุกสถานะ จึงอาจมากกว่าผลรวมของ `online_agent_count` และ `offline_agent_count` หากมี Agent สถานะอื่น เช่น `WARNING` หรือ `DISABLED`

## 3. สร้างห้อง

```http
POST /api/rooms/
Content-Type: application/json
```

Request body:

```json
{
  "name": "Computer Lab 1",
  "description": "ห้องปฏิบัติการชั้น 2"
}
```

`description` สามารถส่งเป็น `null` หรือไม่ส่งมาก็ได้

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/rooms/ \
  -H "Content-Type: application/json" \
  -d '{"name":"Computer Lab 1","description":"ห้องปฏิบัติการชั้น 2"}'
```

Response สำเร็จ: HTTP `201 Created`

```json
{
  "message": "สร้างห้องสำเร็จ",
  "id": "11111111-1111-1111-1111-111111111111"
}
```

## 4. แก้ไขห้อง

```http
PUT /api/rooms/:id
Content-Type: application/json
```

Request body:

```json
{
  "name": "Computer Lab A",
  "description": "ห้องปฏิบัติการที่ปรับปรุงแล้ว"
}
```

```bash
curl -i -b cookies.txt \
  -X PUT http://localhost:8080/api/rooms/11111111-1111-1111-1111-111111111111 \
  -H "Content-Type: application/json" \
  -d '{"name":"Computer Lab A","description":"ห้องปฏิบัติการที่ปรับปรุงแล้ว"}'
```

Response สำเร็จ: HTTP `200 OK`

```json
{
  "message": "แก้ไขห้องสำเร็จ"
}
```

PUT เป็นการแทนค่าข้อมูลห้องทั้งรายการ จึงต้องส่ง `name` ทุกครั้ง หากไม่ต้องการรายละเอียดให้ส่ง `"description": null`

## 5. ลบห้อง

```http
DELETE /api/rooms/:id
```

```bash
curl -i -b cookies.txt \
  -X DELETE http://localhost:8080/api/rooms/11111111-1111-1111-1111-111111111111
```

Response สำเร็จ: HTTP `204 No Content` และไม่มี response body

หากมี Agent อ้างอิงห้องที่ถูกลบ ค่า `room_id` ของ Agent จะเปลี่ยนเป็น `null` ตาม Foreign Key `ON DELETE SET NULL`

## รหัสข้อผิดพลาด

| HTTP Status | กรณี | ตัวอย่าง Response |
| --- | --- | --- |
| `400 Bad Request` | UUID หรือ request body ไม่ถูกต้อง, ไม่ระบุชื่อ หรือชื่อยาวเกินกำหนด | `{"error":"รูปแบบข้อมูลไม่ถูกต้อง"}` |
| `401 Unauthorized` | ยังไม่ได้ Login, cookie ไม่มี หรือ session หมดอายุ | `{"error":"ไม่ได้รับอนุญาตให้เข้าใช้งาน"}` |
| `403 Forbidden` | Role ไม่มี Permission `rooms.manage` | `{"error":"ไม่มีสิทธิ์ดำเนินการนี้"}` |
| `404 Not Found` | ไม่พบห้องตาม ID | `{"error":"ไม่พบห้อง"}` |
| `409 Conflict` | ชื่อห้องซ้ำ | `{"error":"ชื่อห้องนี้มีอยู่แล้ว"}` |
| `500 Internal Server Error` | ไม่สามารถอ่านข้อมูล ตรวจสอบสิทธิ์ หรือดำเนินการกับฐานข้อมูลได้ | `{"error":"ไม่สามารถดำเนินการกับฐานข้อมูลได้"}` |

## ลำดับการตรวจสอบสิทธิ์

ทุก request ของ Rooms API จะถูกตรวจตามลำดับนี้:

1. ตรวจสอบ session cookie และสถานะผู้ใช้
2. อ่าน Role ปัจจุบันของผู้ใช้
3. ตรวจว่า Role เชื่อมกับ Permission `rooms.manage`
4. เรียก Rooms handler เมื่อผ่านการตรวจสอบทั้งหมด

การเพิ่มหรือถอน Permission ของ Role มีผลกับ request ครั้งถัดไปทันที โดยผู้ใช้ไม่จำเป็นต้อง Login ใหม่
