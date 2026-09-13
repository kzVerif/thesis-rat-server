# Files API สำหรับ Frontend

## ข้อมูลทั่วไป

- Base URL: `http://localhost:8080`
- ทุก endpoint ต้อง Login และส่ง session cookie `__Host-session`
- Server เก็บไฟล์ใน `UPLOAD_DIR` หรือโฟลเดอร์ `uploads` หากไม่ได้ตั้งค่า
- ขนาดไฟล์สูงสุด 100 MiB
- Server อ่าน multipart และเขียนลง disk แบบ stream จึงไม่เก็บไฟล์ทั้งก้อนไว้ใน RAM
- ชื่อที่จัดเก็บจะเป็น `<ชื่อ>-<UUID>.<นามสกุล>` เสมอเพื่อไม่ให้ไฟล์ชื่อซ้ำทับกัน
- Frontend ต้องใช้ค่า `filename` ที่ได้รับจาก Server สำหรับการเปลี่ยนชื่อและลบ ห้ามใช้ `original_name`
- รายการและ metadata อ้างอิงจากตาราง `files`; ไฟล์บน disk ที่ไม่มี record ใน DB จะไม่แสดงในระบบ
- Server คำนวณ SHA-256 ระหว่างอัปโหลด และบันทึกผู้ใช้จาก session ลง `uploaded_by`

## Endpoint

| Method | Path | ใช้งาน |
| --- | --- | --- |
| `GET` | `/api/files/?page=1&limit=20` | ดูรายการและ metadata ของไฟล์ |
| `POST` | `/api/files/upload` | อัปโหลดไฟล์แบบ `multipart/form-data` |
| `PATCH` | `/api/files/:filename` | เปลี่ยนชื่อไฟล์ โดย Server เติม UUID ใหม่ให้ |
| `DELETE` | `/api/files/:filename` | ลบไฟล์ออกจากระบบ |

Permission ที่ใช้:

| Endpoint | Permission |
| --- | --- |
| ดูรายการและเปลี่ยนชื่อ | `files.manage` |
| อัปโหลด | `files.upload` หรือ `files.manage` |
| ลบ | `files.delete` หรือ `files.manage` |

## โครงสร้างข้อมูลไฟล์

| Field | รายละเอียด |
| --- | --- |
| `id` | UUID ของรายการไฟล์ ใช้อ้างอิงในข้อมูลของ Frontend/Backend |
| `original_name` | ชื่อดั้งเดิมที่ได้รับตอนอัปโหลดและจะไม่เปลี่ยนเมื่อ rename |
| `filename` | ชื่อปัจจุบันบน Server ใช้เป็น path parameter สำหรับ rename/delete |
| `content_type` | MIME type ที่ได้รับ เช่น `application/pdf` หรือ `image/png` |
| `extension` | นามสกุลของชื่อปัจจุบัน เช่น `.pdf` |
| `file_size` | ขนาดไฟล์หน่วย byte ตรงกับ `files.file_size` |
| `hash_sha256` | SHA-256 ของเนื้อหาไฟล์ ใช้ตรวจสอบความถูกต้อง |
| `uploaded_by` | UUID ของผู้ใช้อัปโหลด |
| `uploader` | username ของผู้อัปโหลด |
| `created_at` | วันเวลาที่อัปโหลดจากฐานข้อมูล |

## 1. ดูรายการไฟล์

```http
GET /api/files/?page=1&limit=20
```

`page` เริ่มต้นที่ 1 และ `limit` เริ่มต้นที่ 20/สูงสุด 100 รายการ เรียงจากไฟล์ที่อัปโหลดล่าสุดก่อน

Response: `200 OK`

```json
{
  "files": [
    {
      "id": "e8d86f31-f3bd-4724-a11d-71a9341fb74c",
      "original_name": "report.pdf",
      "filename": "report-550e8400-e29b-41d4-a716-446655440000.pdf",
      "content_type": "application/pdf",
      "extension": ".pdf",
      "file_size": 42891,
      "hash_sha256": "4f7f2b...64-characters...",
      "uploaded_by": "be808ed9-e820-46a8-a0d9-b3d1dd2defa1",
      "uploader": "admin",
      "created_at": "2026-08-31T08:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 20,
    "total": 1,
    "total_pages": 1
  }
}
```

ตัวอย่าง JavaScript:

```js
async function listFiles(page = 1, limit = 20) {
  const response = await fetch(
    `http://localhost:8080/api/files/?page=${page}&limit=${limit}`,
    { credentials: "include" }
  );
  const data = await response.json();
  if (!response.ok) throw new Error(data.error);
  return data;
}
```

## 2. อัปโหลดไฟล์

ส่ง `multipart/form-data` โดยชื่อ field ต้องเป็น `file` และส่งครั้งละหนึ่งไฟล์

```bash
curl -i -b cookies.txt \
  -X POST http://localhost:8080/api/files/upload \
  -F "file=@./report.pdf"
```

Response: `201 Created`

```json
{
  "message": "อัปโหลดไฟล์สำเร็จ",
  "file": {
    "id": "e8d86f31-f3bd-4724-a11d-71a9341fb74c",
    "original_name": "report.pdf",
    "filename": "report-550e8400-e29b-41d4-a716-446655440000.pdf",
    "content_type": "application/pdf",
    "extension": ".pdf",
    "file_size": 42891,
    "hash_sha256": "4f7f2b...64-characters...",
    "uploaded_by": "be808ed9-e820-46a8-a0d9-b3d1dd2defa1",
    "uploader": "admin",
    "created_at": "2026-08-31T08:30:00Z"
  }
}
```

ตัวอย่าง JavaScript:

```js
async function uploadFile(file) {
  const form = new FormData();
  form.append("file", file);

  const response = await fetch("http://localhost:8080/api/files/upload", {
    method: "POST",
    credentials: "include",
    body: form,
  });

  const data = await response.json();
  if (!response.ok) throw new Error(data.error);
  return data; // บันทึก data.file.filename และ data.file.id ไว้ใช้งานครั้งต่อไป
}
```

> ไม่ต้องกำหนด header `Content-Type` เอง เพราะ browser ต้องสร้าง multipart boundary ให้ตรงกับ body

## 3. เปลี่ยนชื่อไฟล์

ชื่อใน URL คือ `filename` ที่ Server คืนจากการอัปโหลด ส่วน `name` คือชื่อใหม่ที่ต้องการ Server จะเติม UUID ชุดใหม่ให้โดยอัตโนมัติ

นามสกุลต้องตรงกับไฟล์ปัจจุบันทุกตัวและไม่สามารถเปลี่ยนผ่าน rename ได้ เช่น `.pdf` ต้องคงเป็น `.pdf` หากส่ง `.PDF`, `.docx` หรือไม่ส่งนามสกุล API จะตอบ `400 Bad Request`

กฎชื่อไฟล์สำหรับ upload และ rename:

- รองรับ Unicode และชื่อภาษาไทย โดยชื่อ UTF-8 ต้องไม่เกิน 255 bytes
- ห้ามมี path เช่น `/`, `\`, `.` หรือ `..`
- ห้ามขึ้นต้นด้วยจุด เว้นวรรค หรือจบด้วยจุด/เว้นวรรค
- ห้าม control character และอักขระ `< > : " | ? *`
- ห้ามชื่อสงวนของระบบ เช่น `CON`, `PRN`, `AUX`, `NUL`, `CLOCK$`, `COM1`–`COM9` และ `LPT1`–`LPT9`

```bash
curl -i -b cookies.txt \
  -X PATCH "http://localhost:8080/api/files/report-550e8400-e29b-41d4-a716-446655440000.pdf" \
  -H "Content-Type: application/json" \
  -d '{"name":"monthly-report.pdf"}'
```

Response: `200 OK`

```json
{
  "message": "เปลี่ยนชื่อไฟล์สำเร็จ",
  "file": {
    "id": "e8d86f31-f3bd-4724-a11d-71a9341fb74c",
    "original_name": "report.pdf",
    "filename": "monthly-report-7d444840-9dc0-11d1-b245-5ffdce74fad2.pdf",
    "content_type": "application/pdf",
    "extension": ".pdf",
    "file_size": 42891,
    "hash_sha256": "4f7f2b...64-characters...",
    "uploaded_by": "be808ed9-e820-46a8-a0d9-b3d1dd2defa1",
    "uploader": "admin",
    "created_at": "2026-08-31T08:30:00Z"
  }
}
```

Frontend ต้องแทนค่า filename เดิมด้วยค่าที่ได้จาก response

```js
async function renameFile(filename, name) {
  const response = await fetch(
    `http://localhost:8080/api/files/${encodeURIComponent(filename)}`,
    {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    }
  );
  const data = await response.json();
  if (!response.ok) throw new Error(data.error);
  return data;
}
```

## 4. ลบไฟล์

```bash
curl -i -b cookies.txt \
  -X DELETE "http://localhost:8080/api/files/monthly-report-7d444840-9dc0-11d1-b245-5ffdce74fad2.pdf"
```

สำเร็จตอบ `204 No Content` และไม่มี response body

```js
async function deleteFile(filename) {
  const response = await fetch(
    `http://localhost:8080/api/files/${encodeURIComponent(filename)}`,
    { method: "DELETE", credentials: "include" }
  );
  if (!response.ok) {
    const data = await response.json();
    throw new Error(data.error);
  }
}
```

## Error response

Error ทุกกรณีที่มี body ใช้รูปแบบ:

```json
{"error":"รายละเอียดข้อผิดพลาด"}
```

| Status | กรณี |
| --- | --- |
| `400 Bad Request` | multipart ไม่ถูกต้อง, ไม่มี field `file`, หรือชื่อไฟล์ไม่ปลอดภัย |
| `401 Unauthorized` | ไม่มี session หรือ session หมดอายุ |
| `404 Not Found` | ไม่พบไฟล์ที่ต้องการเปลี่ยนชื่อหรือลบ |
| `413 Payload Too Large` | ไฟล์เกิน 100 MiB |
| `500 Internal Server Error` | ไม่สามารถอ่าน เขียน เปลี่ยนชื่อ หรือลบไฟล์บน disk ได้ |

## การตั้งค่า Server

PowerShell:

```powershell
$env:UPLOAD_DIR = "D:\data\rat-uploads"
go run .
```

ควรใช้โฟลเดอร์ที่ process ของ Server มีสิทธิ์อ่านและเขียน และสำรองข้อมูลโฟลเดอร์นี้ตามนโยบายของระบบ

หากฐานข้อมูลถูกสร้างไว้ก่อนเพิ่ม Files API ให้รัน `docs/FILES_DB_MIGRATION.sql` หนึ่งครั้ง เพื่อเพิ่ม permission, unique index และสิทธิ์ของ Administrator
