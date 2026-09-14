# Dashboard API

## อ่านข้อมูล Dashboard

```http
GET /api/dashboard/
```

ต้องเข้าสู่ระบบด้วย cookie `__Host-session` บัญชีต้องมีสถานะ `ACTIVE` ไม่ต้องใช้ permission เพิ่มเติมนอกเหนือจากการ login

เส้นนี้คืนค่า aggregate สำหรับหน้า Dashboard ใน request เดียว ไม่มีรายการผู้ใช้ รายการ Agent หรือข้อมูลลับจาก token จึงเหมาะสำหรับเรียกตอนเปิดหน้าและ refresh เป็นช่วง ๆ

ตัวอย่าง response:

```json
{
  "generated_at": "2026-09-14T10:00:00Z",
  "agents": {
    "total": 20,
    "online": 12,
    "offline": 6,
    "warning": 1,
    "disabled": 1
  },
  "rooms": { "total": 4 },
  "users": {
    "total": 8,
    "active": 6,
    "disabled": 1,
    "locked": 1
  },
  "tokens": {
    "total": 10,
    "active": 5,
    "revoked": 2,
    "expired": 1,
    "exhausted": 2,
    "uses": 37
  },
  "files": {
    "total": 42,
    "total_bytes": 734003200
  },
  "antivirus": {
    "total_scans": 18,
    "completed": 15,
    "failed": 2,
    "threats_found": 3
  },
  "file_distributions": {
    "total": 12,
    "pending": 2,
    "in_progress": 3,
    "completed": 6,
    "failed": 1
  },
  "activity": { "last_24_hours": 128 }
}
```

## ความหมายของข้อมูล

- `generated_at`: เวลาที่ฐานข้อมูลสร้าง snapshot
- `agents`: จำนวน Agent แยกตาม `ONLINE`, `OFFLINE`, `WARNING`, `DISABLED`
- `rooms.total`: จำนวนห้องทั้งหมด
- `users`: จำนวนผู้ใช้แยกตาม `ACTIVE`, `DISABLED`, `LOCKED`
- `tokens.active`: token ที่ยังไม่ถูกยุติ ไม่หมดอายุ และยังไม่ใช้ครบ quota
- `tokens.expired`: token ที่หมดอายุและยังไม่ได้ revoke
- `tokens.exhausted`: token ที่ใช้ครบ `max_use`
- `tokens.uses`: จำนวนการลงทะเบียน Agent สำเร็จสะสมจาก `used_count`
- `files.total_bytes`: ขนาดไฟล์รวมเป็น bytes
- `antivirus`: จำนวนผลสแกนและจำนวนภัยคุกคามที่พบ
- `file_distributions`: จำนวนงานกระจายไฟล์แยกตามสถานะ
- `activity.last_24_hours`: จำนวน audit logs ที่สร้างใน 24 ชั่วโมงล่าสุด

## การเรียกจาก Frontend

```js
const response = await fetch('/api/dashboard/', {
  method: 'GET',
  credentials: 'include',
});

const dashboard = await response.json();
if (!response.ok) throw new Error(dashboard.error);
```

ไม่พบ session หรือ session หมดอายุ: `401` ถ้าฐานข้อมูลอ่านไม่ได้: `500` response error ใช้รูปแบบ `{ "error": "..." }`
