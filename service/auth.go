package service

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const (
	hostCookieName = "__Host-session"
	devCookieName  = "session"
	sessionTTL     = 7 * 24 * time.Hour
	authUserLocal  = "authUser"
)

const AdministratorRoleID = "be808ed9-e820-46a8-a0d9-b3d1dd2defa1"

type authUser struct {
	ID, SessionID, Username, RoleID, Role, PasswordHash string
	Email, DisplayName                                  sql.NullString
}

type RegisterData struct {
	Username        string `json:"username"`
	Fullname        string `json:"fullname"`
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

func CreateUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user := new(RegisterData)
		if err := c.BodyParser(user); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "รูปแบบข้อมูลที่ส่งมาไม่ถูกต้อง"})
		}

		user.Username = strings.TrimSpace(user.Username)
		user.Fullname = strings.TrimSpace(user.Fullname)
		user.Email = strings.ToLower(strings.TrimSpace(user.Email))
		if user.Username == "" || user.Fullname == "" || user.Email == "" || user.Password == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "กรุณากรอกข้อมูลให้ครบทุกช่อง"})
		}
		if user.Password != user.ConfirmPassword {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "รหัสผ่านและการยืนยันรหัสผ่านไม่ตรงกัน"})
		}
		if len(user.Password) < 8 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "รหัสผ่านต้องมีความยาวอย่างน้อย 8 ตัวอักษร"})
		}

		passwordHash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ไม่สามารถเข้ารหัสรหัสผ่านได้"})
		}

		var userID string
		err = db.QueryRow(`
			INSERT INTO users (username, email, password_hash, display_name, role_id, status)
			SELECT $1, $2, $3, $4, id, 'DISABLED' FROM roles WHERE name = 'VIEWER'
			RETURNING id
		`, user.Username, user.Email, string(passwordHash), user.Fullname).Scan(&userID)
		if err != nil {
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรืออีเมลนี้ถูกใช้งานแล้ว"})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "ไม่สามารถสร้างผู้ใช้ได้ กรุณาตรวจสอบว่ามีบทบาท VIEWER ในระบบ",
			})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message": "สร้างผู้ใช้สำเร็จ", "user_id": userID,
			"username": user.Username, "email": user.Email, "fullname": user.Fullname,
		})
	}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isSecureConnection(c *fiber.Ctx) bool {
	return c.Protocol() == "https" || c.Get("X-Forwarded-Proto") == "https"
}

func writeCookie(c *fiber.Ctx, token string, expires time.Time) {
	secure := isSecureConnection(c)
	name := devCookieName
	if secure {
		name = hostCookieName
	}
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HTTPOnly: true,
		Secure:   secure,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

func removeCookie(c *fiber.Ctx) {
	for _, name := range []string{devCookieName, hostCookieName} {
		c.Cookie(&fiber.Cookie{
			Name:     name,
			Path:     "/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HTTPOnly: true,
			Secure:   false,
			SameSite: fiber.CookieSameSiteLaxMode,
		})
	}
}

func authenticate(db *sql.DB, c *fiber.Ctx) (*authUser, error) {
	token := c.Cookies(hostCookieName)
	if token == "" {
		token = c.Cookies(devCookieName)
	}
	if token == "" {
		removeCookie(c)
		return nil, c.Status(401).JSON(fiber.Map{"error": "ไม่ได้รับอนุญาตให้เข้าใช้งาน"})
	}
	u := new(authUser)
	err := db.QueryRow(`SELECT u.id,s.id,u.username,u.email,u.display_name,r.id,r.name,u.password_hash
		FROM user_sessions s JOIN users u ON u.id=s.user_id JOIN roles r ON r.id=u.role_id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW() AND u.status='ACTIVE'`,
		hashToken(token)).Scan(&u.ID, &u.SessionID, &u.Username, &u.Email, &u.DisplayName, &u.RoleID, &u.Role, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		removeCookie(c)
		return nil, c.Status(401).JSON(fiber.Map{"error": "ไม่ได้รับอนุญาตให้เข้าใช้งาน"})
	}
	if err != nil {
		return nil, c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถตรวจสอบเซสชันได้"})
	}
	_, _ = db.Exec(`UPDATE user_sessions SET last_activity_at=NOW() WHERE id=$1`, u.SessionID)
	return u, nil
}

// RequireAuth rejects requests without a valid, active session and makes the
// authenticated user available to handlers through Fiber Locals.
func RequireAuth(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, err := authenticate(db, c)
		if err != nil {
			return err
		}
		c.Locals(authUserLocal, u)
		return c.Next()
	}
}

// RequireAdministrator must be registered after RequireAuth.
func RequireAdministrator(c *fiber.Ctx) error {
	u, ok := c.Locals(authUserLocal).(*authUser)
	if !ok || u == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "ไม่ได้เข้าสู่ระบบ"})
	}
	if u.RoleID != AdministratorRoleID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "ต้องเป็นผู้ดูแลระบบเท่านั้น"})
	}
	return c.Next()
}

func signedInUser(c *fiber.Ctx) *authUser {
	return c.Locals(authUserLocal).(*authUser)
}

func Login(db *sql.DB) fiber.Handler {
	type request struct{ Username, Password string }
	return func(c *fiber.Ctx) error {
		var in request
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Username = strings.TrimSpace(in.Username)
		if in.Username == "" || in.Password == "" {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณากรอกชื่อผู้ใช้และรหัสผ่าน"})
		}
		var id, username, passwordHash, status string
		err := db.QueryRow(`SELECT id,username,password_hash,status FROM users WHERE username=$1`, in.Username).
			Scan(&id, &username, &passwordHash, &status)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(in.Password)) != nil) {
			return c.Status(401).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรือรหัสผ่านไม่ถูกต้อง"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเข้าสู่ระบบได้"})
		}
		if status != "ACTIVE" {
			return c.Status(403).JSON(fiber.Map{"error": "บัญชีนี้ไม่สามารถใช้งานได้"})
		}
		token, err := randomToken()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถสร้างเซสชันได้"})
		}
		expires := time.Now().Add(sessionTTL)
		var sessionID string
		err = db.QueryRow(`INSERT INTO user_sessions(user_id,token_hash,ip_address,user_agent,expires_at)
			VALUES($1,$2,NULLIF($3,'')::inet,$4,$5) RETURNING id`, id, hashToken(token), c.IP(),
			string(c.Request().Header.UserAgent()), expires).Scan(&sessionID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถสร้างเซสชันได้"})
		}
		_, _ = db.Exec(`UPDATE users SET last_login_at=NOW(),failed_login_attempts=0 WHERE id=$1`, id)
		writeCookie(c, token, expires)
		return c.JSON(fiber.Map{"message": "เข้าสู่ระบบสำเร็จ", "session_id": sessionID, "username": username})
	}
}

func Logout(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		var err error
		if _, err = db.Exec(`UPDATE user_sessions SET revoked_at=NOW(),revoked_reason='logout' WHERE id=$1`, u.SessionID); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถออกจากระบบได้"})
		}
		removeCookie(c)
		return c.JSON(fiber.Map{"message": "ออกจากระบบสำเร็จ"})
	}
}

func LogoutAll(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		var err error
		_, err = db.Exec(`UPDATE user_sessions SET revoked_at=NOW(),revoked_reason='logout-all' WHERE user_id=$1 AND revoked_at IS NULL`, u.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถออกจากระบบทุกอุปกรณ์ได้"})
		}
		removeCookie(c)
		return c.JSON(fiber.Map{"message": "ออกจากระบบทุกอุปกรณ์สำเร็จ"})
	}
}

func Me(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		return c.JSON(fiber.Map{"id": u.ID, "username": u.Username, "email": u.Email.String,
			"display_name": u.DisplayName.String, "role_id": u.RoleID, "role": u.Role})
	}
}

func Sessions(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		rows, err := db.Query(`SELECT id,COALESCE(host(ip_address),''),COALESCE(user_agent,''),created_at,
			last_activity_at,expires_at,id=$2 FROM user_sessions
			WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>NOW() ORDER BY last_activity_at DESC`, u.ID, u.SessionID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการเซสชันได้"})
		}
		defer rows.Close()
		out := make([]fiber.Map, 0)
		for rows.Next() {
			var id, ip, agent string
			var created, active, expires time.Time
			var current bool
			if rows.Scan(&id, &ip, &agent, &created, &active, &expires, &current) != nil {
				return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการเซสชันได้"})
			}
			out = append(out, fiber.Map{"id": id, "ip_address": ip, "user_agent": agent,
				"created_at": created, "last_activity_at": active, "expires_at": expires, "current": current})
		}
		if rows.Err() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการเซสชันได้"})
		}
		return c.JSON(fiber.Map{"sessions": out})
	}
}

func DeleteSession(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		result, err := db.Exec(`UPDATE user_sessions SET revoked_at=NOW(),revoked_reason='revoked-by-user'
			WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, c.Params("id"), u.ID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถยกเลิกเซสชันได้"})
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบเซสชัน"})
		}
		if c.Params("id") == u.SessionID {
			removeCookie(c)
		}
		return c.JSON(fiber.Map{"message": "ยกเลิกเซสชันสำเร็จ"})
	}
}

func ChangePassword(db *sql.DB) fiber.Handler {
	type request struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
		Confirm string `json:"confirm_password"`
	}
	return func(c *fiber.Ctx) error {
		u := signedInUser(c)
		var err error
		var in request
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Current)) != nil {
			return c.Status(401).JSON(fiber.Map{"error": "รหัสผ่านปัจจุบันไม่ถูกต้อง"})
		}
		if len(in.New) < 8 {
			return c.Status(400).JSON(fiber.Map{"error": "รหัสผ่านใหม่ต้องมีอย่างน้อย 8 ตัวอักษร"})
		}
		if in.New != in.Confirm {
			return c.Status(400).JSON(fiber.Map{"error": "รหัสผ่านใหม่และการยืนยันไม่ตรงกัน"})
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.New)) == nil {
			return c.Status(400).JSON(fiber.Map{"error": "รหัสผ่านใหม่ต้องไม่ซ้ำกับรหัสผ่านเดิม"})
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.New), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนรหัสผ่านได้"})
		}
		tx, err := db.Begin()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนรหัสผ่านได้"})
		}
		defer tx.Rollback()
		_, err = tx.Exec(`UPDATE users SET password_hash=$1,password_changed_at=NOW(),updated_at=NOW() WHERE id=$2`, string(hash), u.ID)
		if err == nil {
			_, err = tx.Exec(`UPDATE user_sessions SET revoked_at=NOW(),revoked_reason='password-changed' WHERE user_id=$1 AND revoked_at IS NULL`, u.ID)
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนรหัสผ่านได้"})
		}
		if err = tx.Commit(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนรหัสผ่านได้"})
		}
		removeCookie(c)
		return c.JSON(fiber.Map{"message": "เปลี่ยนรหัสผ่านสำเร็จ กรุณาเข้าสู่ระบบใหม่"})
	}
}
