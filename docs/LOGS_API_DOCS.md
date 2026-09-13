# Logs API

ทุก endpoint ต้องเข้าสู่ระบบด้วย session cookie เดิม และมี permission `logs.read` (ไม่มีสิทธิ์: 403, ไม่ได้เข้าสู่ระบบ: 401)

## อ่านรายการ

`GET /api/logs?page=1&limit=20`

| Query | ความหมาย |
| --- | --- |
| `page` | เริ่มที่ 1, default 1 |
| `limit` | 1–100, default 20 |
| `user_id` | UUID ผู้กระทำ |
| `target_agent_id` | UUID เครื่องเป้าหมาย |
| `action` | ตรงกับ action แบบเต็ม เช่น `POST /api/rooms/` |
| `from` | เวลาเริ่มต้น RFC3339 รวมเวลาที่ระบุ |
| `to` | เวลาสิ้นสุด RFC3339 รวมเวลาที่ระบุ |

เรียง `created_at DESC, id DESC` โดย count และรายการในแต่ละ request ใช้ snapshot เดียวกัน ข้อมูลใหม่อาจทำให้ตำแหน่งเปลี่ยนเมื่อเปลี่ยนหน้า ใช้ `to` คงที่เมื่อต้องการดูช่วงเวลาเดิม

```json
{
  "logs": [
    {
      "id": "ec3bd54d-5d28-4ea6-aaf8-0a3916fdf08a",
      "user_id": "bca376bb-7e48-4d75-afaa-e6b75b597c60",
      "username": "admin",
      "display_name": "Administrator",
      "action": "POST /api/rooms/",
      "target_agent_id": null,
      "agent_hostname": null,
      "detail": {
        "method": "POST",
        "route": "/api/rooms/",
        "status_code": 201,
        "success": true,
        "duration_ms": 12,
        "actor_id": "bca376bb-7e48-4d75-afaa-e6b75b597c60",
        "actor_username": "admin",
        "resource_id": "46d6dbf1-0419-4d78-80fa-27d29d062f12"
      },
      "ip_address": "127.0.0.1",
      "created_at": "2026-09-13T10:00:00Z"
    }
  ],
  "pagination": { "page": 1, "limit": 20, "total": 1, "total_pages": 1 }
}
```

ไม่พบรายการ: `logs: []`, `total: 0`, `total_pages: 0` ค่า query ผิดรูปแบบ: 400 ฐานข้อมูลผิดพลาด: 500

## อ่านรายการเดียว

`GET /api/logs/:id` คืน object ของ log ตามตัวอย่างด้านบนโดยไม่มี wrapper; UUID ผิดรูปแบบ: 400, ไม่พบ: 404

## ตัวอย่าง frontend

```js
const query = new URLSearchParams({ page: '1', limit: '20' });
// query.set('action', 'POST /api/rooms/');
// query.set('from', new Date('2026-09-13T00:00:00+07:00').toISOString());
const response = await fetch(`/api/logs?${query}`, { credentials: 'include' });
const data = await response.json();
if (!response.ok) throw new Error(data.error);
// data.logs: รายการสำหรับตาราง, data.pagination: ข้อมูลแบ่งหน้า
```

## ขอบเขตการบันทึก

- บันทึก `POST`, `PUT`, `PATCH`, `DELETE` ที่ผ่าน middleware `/api` ทั้งสำเร็จและล้มเหลว: สมัครสมาชิก, login/logout, เปลี่ยนรหัสผ่าน, ยกเลิก session, สร้าง/แก้ไข/ลบ users, roles, permissions, rooms, agents และอัปโหลด/เปลี่ยนชื่อ/ลบไฟล์
- ไม่บันทึก GET/HEAD/OPTIONS ทั่วไป รวม polling dashboard และการอ่าน logs เพื่อไม่ให้การเปิดหน้า frontend สร้าง logs ต่อเนื่อง
- ทุก method ยังคงบันทึกสถานะ `401`, `403`, `429`, `5xx` เพื่อเก็บเหตุการณ์ไม่ได้เข้าสู่ระบบ/ไม่มีสิทธิ์/เรียกถี่เกิน/ระบบผิดพลาด ส่วน GET ที่ตอบ 400 หรือ 404 ไม่บันทึก
- สำหรับสถานะตั้งแต่ 400 ขึ้นไป เก็บครั้งแรกต่อชุด (ผู้ใช้, IP, method + route, status) ในหน้าต่าง 1 นาที เหตุการณ์ซ้ำในหน้าต่างเดียวกันจะถูกข้าม ไม่รวมยอดที่ข้าม และไม่แยกตาม target ID จำกัด 4,096 ชุดต่อหน้าต่าง; เมื่อเต็มจะข้ามชุดใหม่จนหน้าต่างถัดไป เป็นตัวจำกัดในหน่วยความจำต่อ server process และเริ่มใหม่เมื่อ restart จึงไม่ใช่ rate limit ป้องกันการเรียก API
- การกระทำสำเร็จไม่ถูกจำกัดจำนวนและไม่ถูกรวมรายการ
- `action` คือ HTTP method + route template ของ Fiber เช่น `PUT /api/agents/:id` ไม่ใช่ URL ที่มี UUID จริง หาก auth หรือ group middleware ปฏิเสธก่อนถึง endpoint จะเป็น route ของ middleware ที่ปฏิเสธ เช่น `GET /api`
- `detail.status_code` คือ HTTP status และ `success` คือ status 200–399 ซึ่งหมายถึงผลของ HTTP request ไม่ใช่ผลการทำงานภายหลังของเครื่องลูก
- เก็บ identifier จาก path (`id`, `jobId`, `filename`) และ identifier จาก response สำเร็จ (`resource_id`, `user_id`, `file_id`) เมื่อมี
- Login/register สำเร็จมีผู้กระทำ; login ล้มเหลวไม่มีผู้กระทำที่ยืนยันตัวตนแล้ว (`user_id: null`)
- ไม่เก็บ request body, query string, cookies, authorization headers, passwords, tokens หรือเนื้อหาไฟล์
- เมื่อ user/agent ถูกลบ FK จะเป็น null แต่ identifier ใน `detail` ยังคงอยู่ ชื่อที่ join เป็นชื่อปัจจุบัน ส่วน `actor_username` เป็นชื่อขณะทำรายการ
- บันทึกหลัง handler ทำงานเสร็จ แยกจาก transaction ของงานหลัก หากเขียน log ไม่สำเร็จจะแจ้ง server log โดยคง response เดิม มี timeout การเขียน 3 วินาที จึงไม่รับประกัน audit แบบ atomic หรือกรณี process หยุดกลาง request
- ไม่ครอบคลุมการแก้ไขฐานข้อมูลโดยโปรแกรมอื่นหรือ background jobs ที่ไม่ผ่าน HTTP server นี้

## ฐานข้อมูลเดิม

รัน `migrations/20260913_logs.sql` หนึ่งครั้งเพื่อเพิ่ม permission ให้ ADMINISTRATOR และ index สำหรับกรอง action (รันซ้ำได้) โดยต้องมีตาราง logs ตาม schema อยู่แล้ว

ฐานข้อมูลใหม่ใช้ `schema.sql` และ `seed.sql` ตามเดิม; seed มี `logs.read` และให้สิทธิ์ ADMINISTRATOR อยู่แล้ว บทบาทอื่นให้สิทธิ์ผ่าน API จัดการ roles

## อายุการเก็บและลบอัตโนมัติ

- ค่าเริ่มต้น `LOG_RETENTION_DAYS=90` ตั้งผ่าน environment ของ process เช่นเดียวกับ DB settings (ไฟล์ `.env.example` เป็นตัวอย่าง ไม่ได้ถูกโหลดอัตโนมัติโดยโค้ดนี้) ปรับได้ตั้งแต่ 1–3650 วัน; ค่าผิดทำให้ server ไม่เริ่มทำงาน
- Worker เริ่มเมื่อเปิด server และทำซ้ำทุก 24 ชั่วโมง ลบเฉพาะแถวที่ `created_at` เก่ากว่าเวลาปัจจุบันลบจำนวนวันที่กำหนด รวม logs เดิมทุกประเภท จึงอาจมีแถวอายุเกิน 90 วันระหว่างรอบหรือเมื่อ server ปิด
- ลบครั้งละไม่เกิน 1,000 แถว เว้น 100 ms ระหว่างชุด จำกัดแต่ละคำสั่ง 10 วินาทีและรอบละ 5 นาที ใช้ index `idx_logs_created_at` เดิมและข้ามแถวที่ถูกล็อก
- แต่ละชุด commit แยกกัน หากผิดพลาด/หมดเวลา จะรายงาน server log และทำต่อในรอบวันถัดไป หาก backlog มากอาจต้องใช้หลายรอบ
- ลบถาวร ไม่มีการ archive อัตโนมัติ; งานลบนี้ไม่สร้าง audit row เพิ่ม รายงานจำนวนที่ลบใน server log
- ไม่ต้องเพิ่ม schema สำหรับ retention; การลบช่วยให้ PostgreSQL นำพื้นที่กลับมาใช้ซ้ำเมื่อ vacuum ทำงาน แต่ขนาดไฟล์บนดิสก์อาจไม่ลดทันที และไม่ใช่การจำกัดขนาดฐานข้อมูลแบบตายตัว
