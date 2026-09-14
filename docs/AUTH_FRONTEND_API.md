# คู่มือ Auth API สำหรับ Frontend

ใช้ prefix `/api/auth` สำหรับทุก action ที่เกี่ยวกับการสมัครและ session เท่านั้น ห้ามใช้ `/api/users/register`

ทุก request ที่มี body ต้องส่ง `Content-Type: application/json` และใช้ `credentials: 'include'` เพื่อให้ browser ส่ง cookie `__Host-session`

`__Host-session` ต้องใช้ผ่าน HTTPS เท่านั้นตามข้อกำหนดของ cookie prefix ดังนั้น local ต้องเปิด HTTPS หรือใช้ reverse proxy HTTPS หน้า backend

ถ้า frontend กับ backend ใช้คนละ port ให้ส่ง `credentials: 'include'` ทุกครั้ง Backend อนุญาต origin local `localhost:3000`, `localhost:5173`, `127.0.0.1:3000` และ `127.0.0.1:5173` โดยค่าเริ่มต้น หากใช้ port อื่นให้เพิ่ม origin ใน `FRONTEND_ORIGIN` คั่นด้วย comma

## สมัครผู้ใช้

```http
POST /api/auth/register
```

ไม่ต้อง login

```json
{
  "username": "newuser",
  "fullname": "New User",
  "email": "newuser@example.com",
  "password": "strong-password",
  "confirmPassword": "strong-password"
}
```

สำเร็จ `201`:

```json
{
  "message": "สร้างผู้ใช้สำเร็จ",
  "user_id": "uuid",
  "username": "newuser",
  "email": "newuser@example.com",
  "fullname": "New User",
  "status": "DISABLED"
}
```

บัญชีใหม่มีสถานะ `DISABLED` ต้องให้ผู้มีสิทธิ์ `users.manage` เปลี่ยนเป็น `ACTIVE` ก่อนจึง login ได้

## Login

```http
POST /api/auth/login
```

```json
{
  "username": "admin",
  "password": "your-password"
}
```

สำเร็จ `200` และ server จะสร้าง session cookie:

```json
{
  "message": "เข้าสู่ระบบสำเร็จ",
  "session_id": "uuid",
  "username": "admin"
}
```

## ดูข้อมูลผู้ใช้ปัจจุบัน

```http
GET /api/auth/me
```

ต้อง login สำเร็จ

## ดู Session

```http
GET /api/auth/sessions
```

## ยกเลิก Session รายการเดียว

```http
DELETE /api/auth/sessions/:id
```

## Logout

```http
POST /api/auth/logout
```

## Logout ทุกอุปกรณ์

```http
POST /api/auth/logout-all
```

## เปลี่ยนรหัสผ่าน

```http
POST /api/auth/change-password
```

```json
{
  "current_password": "old-password",
  "new_password": "new-password",
  "confirm_password": "new-password"
}
```

เมื่อเปลี่ยนสำเร็จ session เดิมทั้งหมดจะถูกยกเลิกและต้อง login ใหม่

## ตัวอย่างการเรียกจาก Frontend

```js
async function authRequest(path, options = {}) {
  const response = await fetch(`/api/auth${path}`, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
    ...options,
  });

  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `Request failed: ${response.status}`);
  }
  return data;
}

await authRequest('/register', {
  method: 'POST',
  body: JSON.stringify({
    username: 'newuser',
    fullname: 'New User',
    email: 'newuser@example.com',
    password: 'strong-password',
    confirmPassword: 'strong-password',
  }),
});

await authRequest('/login', {
  method: 'POST',
  body: JSON.stringify({ username: 'admin', password: 'your-password' }),
});

const currentUser = await authRequest('/me');
```

สถานะที่ frontend ควรรองรับ:

- `400`: ข้อมูลไม่ครบหรือรูปแบบไม่ถูกต้อง
- `401`: ยังไม่ได้ login หรือ session หมดอายุ
- `403`: บัญชีไม่อยู่ในสถานะที่ใช้งานได้ (`code: ACCOUNT_NOT_ACTIVE`, ตรวจ `status` เป็น `DISABLED` หรือ `LOCKED`)
- `409`: username หรือ email ซ้ำ
- `500`: server หรือฐานข้อมูลทำงานผิดพลาด
