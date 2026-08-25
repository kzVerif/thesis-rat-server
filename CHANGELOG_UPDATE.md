# 📋 เอกสารสรุปการอัปเดตระบบ: Tokens Management Feature & Local Dev Compatibility

เอกสารนี้จัดทำขึ้นเพื่อสรุปการเปลี่ยนแปลงและฟีเจอร์ใหม่ที่พัฒนาเพิ่มขึ้นมาจากโค้ดล่าสุดบน Git เพื่อให้ทีมและผู้พัฒนาเข้าใจสิ่งที่เพิ่มเข้ามา เหตุผลในการปรับปรุง และวิธีการทำงานร่วมกันได้อย่างราบรื่น

---

## 📌 สรุปภาพรวม (Overview)

1. **เพิ่มระบบจัดการ Token (Tokens Management):** รองรับการออก Token สำหรับ Agent / API Key, กำหนดสิทธิ์, จำกัดจำนวนครั้งการใช้งาน (`max_use`), กำหนดวันหมดอายุ (`expires_at`), การระงับการใช้งาน (`is_revoked`) และระบบตรวจสอบ Token (`/api/tokens/validate`)
2. **ระบบ Cookie ปรับให้รองรับทั้ง Localhost (HTTP) และ Production (HTTPS):** ไม่กระทบความปลอดภัยเดิมบน Production และทำให้สามารถรัน/ทดสอบบนเครื่อง Localhost ผ่าน Safari/Chrome ได้โดยไม่ติดปัญหา Cookie หาย
3. **รักษาความเข้ากันได้ 100% (Backward Compatibility):** API เดิมทั้งหมด (`/api/auth`, `/api/roles`, `/api/permissions`, `/api/rooms`, `/api/users`) ยังคงทำงานเหมือนเดิมทุกประการ

---

## 🛠️ รายละเอียดการเปลี่ยนแปลงฝั่ง Backend (`thesis-rat-server`)

### 1. โครงสร้างฐานข้อมูล ([schema.sql](schema.sql))
เพิ่มคำสั่งสร้างตาราง `tokens` ที่ท้ายไฟล์ โดยใช้ `CREATE TABLE IF NOT EXISTS` เพื่อไม่ให้กระทบตารางเดิม:

```sql
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NULL,
    agent_id UUID NULL,
    token TEXT NOT NULL,
    token_type VARCHAR(50) NOT NULL,
    max_use INT DEFAULT NULL,
    used_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_tokens_token UNIQUE (token),
    CONSTRAINT fk_tokens_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_tokens_agent FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_tokens_agent_id ON tokens (agent_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tokens_token_unique ON tokens (token);
```

---

### 2. Service จัดการ Tokens ใหม่ ([service/tokens.go](service/tokens.go))
สร้างไฟล์ใหม่สำหรับ Business Logic ของ Token ทั้งหมด:
* `GET /api/tokens` (`ListTokens`): ดึงรายการ Token ทั้งหมด
* `POST /api/tokens` (`CreateToken`): สร้าง Token สุ่มแบบ URL-safe Base64 พร้อมตั้งค่าเงื่อนไข
* `PUT, PATCH /api/tokens/:id` (`UpdateToken`): แก้ไขเงื่อนไขของ Token
  * **จุดแก้ไขสำคัญ (Bugfix & Flexibility):**
    * รองรับทั้งคีย์ `snake_case` (`max_use`, `expires_at`, `is_revoked`) และ `camelCase` (`maxUse`, `expiresAt`, `isRevoked`) จาก Frontend
    * ถอดรหัสวันหมดอายุรองรับทั้ง `RFC3339` (เช่น `2026-08-20T00:00:00Z`) และ Datepicker format `YYYY-MM-DD` (เช่น `2026-08-25`)
    * หากส่งค่าว่าง `""` หรือ `null` จะถูกแปลงเป็น `NULL` ในฐานข้อมูลทันที (หมายถึงไม่จำกัดการใช้งาน / ไม่มีวันหมดอายุ)
* `DELETE /api/tokens/:id` (`RevokeToken`): ลบหรือยกเลิก Token
* `POST /api/tokens/validate` (`ValidateToken`): ตรวจสอบความถูกต้องของ Token, ตรวจวันหมดอายุ, ตรวจสอบ `used_count` เทียบกับ `max_use` และนับการใช้งานเพิ่มอัตโนมัติ

---

### 3. ปรับปรุง Auth & Session Cookie ([service/auth.go](service/auth.go))
เพื่อแก้ปัญหา Safari / Browser ปฏิเสธ Cookie เมื่อรันบน `http://localhost`:

* **`isSecureConnection(c *fiber.Ctx)`:** ตรวจสอบโปรโตคอลของ Request
  * **ถ้าเป็น HTTPS (Production):** ใช้ชื่อ Cookie ว่า **`__Host-session`** พร้อม **`Secure: true`** (ตรงตามมาตรฐานเดิมบน Git 100%)
  * **ถ้าเป็น HTTP (Localhost):** ใช้ชื่อ Cookie ว่า **`session`** พร้อม **`Secure: false`** เพื่อให้ Browser บันทึก Cookie ได้
* **`authenticate`:** ตรวจสอบได้ทั้ง Cookie ชื่อ `__Host-session` และ `session` จึงรองรับได้ทั้ง 2 ระบบอย่างไร้รอยต่อ

---

### 4. อัปเดต Routing & Middleware ([main.go](main.go))
* **เพิ่ม CORS Middleware:**
  ```go
  app.Use(cors.New(cors.Config{
      AllowOrigins:     "http://localhost:3000, http://localhost:5173, http://127.0.0.1:3000, http://127.0.0.1:5173",
      AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
      AllowCredentials: true,
      AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS, PATCH",
  }))
  ```
* **เพิ่ม Route Group ของ Tokens:**
  ```go
  tokens := api.Group("/tokens", service.RequirePermission(db, service.UsersManagePermission))
  tokens.Get("/", service.ListTokens(db))
  tokens.Post("/", service.CreateToken(db))
  tokens.Put("/:id", service.UpdateToken(db))
  tokens.Patch("/:id", service.UpdateToken(db))
  tokens.Delete("/:id", service.RevokeToken(db))
  tokens.Post("/validate", service.ValidateToken(db))
  ```

---

## 💻 รายละเอียดการเปลี่ยนแปลงฝั่ง Frontend Dashboard (`thesis-rat-dashboard`)

### 1. UI จัดการ Tokens (`app/(system)/tokens/_components/TokenManagement.tsx`)
* หน้าต่างตารางแสดงรายการ Token, สถานะ (Active / Warning / Expired / Used)
* Modal สำหรับ **"สร้าง Token ใหม่"** (รองรับทั้งสร้างรหัสอัตโนมัติ และกำหนดเอง)
* Modal สำหรับ **"แก้ไข Token"** (ปรับจำนวน `max_use` และวันหมดอายุ `expires_at`)
* Modal สำหรับ **"ลบ Token"**

### 2. Client Fetcher (`app/(system)/tokens/_lib/tokensClient.ts`)
* ฟังก์ชันเรียก API ฝั่ง Client: `listTokens()`, `createToken()`, `updateToken()`, `revokeToken()` พร้อมแนบ `credentials: "include"`

### 3. API Proxy Route (`app/api/tokens/[[...path]]/route.ts`)
* Next.js Route Handler ทำหน้าที่ Proxy ส่ง Request จากหน้าบ้านไปยัง Go Backend (`http://localhost:8080/api/tokens/*`) พร้อมส่งต่อ Cookies และ Headers ให้ครบถ้วน

### 4. Auth Utility (`lib/auth.ts`)
* ปรับ `authFetch` ให้อ่าน Session Cookie ได้ทั้ง `__Host-session` และ `session`:
  ```typescript
  const cookieStore = await cookies();
  const session = cookieStore.get("__Host-session") || cookieStore.get("session");
  ```

---

## 🚀 ขั้นตอนการติดตั้งและรันสำหรับผู้ร่วมพัฒนา (How to Run / Migrate)

### 1. อัปเดตฐานข้อมูล (Database)
หากมีฐานข้อมูลเดิมอยู่แล้ว ให้รันคำสั่ง SQL สร้างตาราง `tokens` (อยู่ใน `schema.sql` ท้ายไฟล์):
```bash
docker exec -it ratsystem-postgres psql -U postgres -d ratsystem -c "
CREATE TABLE IF NOT EXISTS tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NULL,
    agent_id UUID NULL,
    token TEXT NOT NULL UNIQUE,
    token_type VARCHAR(50) NOT NULL,
    max_use INT DEFAULT NULL,
    used_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ,
    is_revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
"
```

### 2. รัน Backend
```bash
cd thesis-rat-server
go mod tidy
go build -o rat-server .
./rat-server
```

### 3. รัน Frontend Dashboard
```bash
cd thesis-rat-dashboard
npm install
npm run dev
```

---

## ❓ คำถามที่พบบ่อย (FAQ & Notes)

* **Q: ถ้าเอาขึ้น Production ที่มี HTTPS ระบบจะยังปลอดภัยเหมือนเดิมไหม?**
  * **A:** ปลอดภัยเหมือนเดิม 100% เพราะฟังก์ชัน `writeCookie` จะตรวจพบว่าการเชื่อมต่อเป็น HTTPS และจะสลับไปใช้ `__Host-session` พร้อม `Secure: true` โดยอัตโนมัติ
* **Q: การแก้ไขค่า `max_use` ทำไมต้องรองรับทั้ง `""` และ `null`?**
  * **A:** เพราะใน UI ถ้าผู้ใช้ลบตัวเลขในกล่องข้อความเพื่อต้องการให้ใช้ได้ไม่จำกัด (Unlimited) ค่าจะถูกส่งมาเป็น Empty String `""` ซึ่ง Backend ได้จัดการแปลงให้เป็น `NULL` ใน Database โดยไม่เกิด Error แล้ว
