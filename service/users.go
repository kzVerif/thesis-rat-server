package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const UsersManagePermission = "users.manage"

type userInput struct {
	Username    string  `json:"username"`
	Email       *string `json:"email"`
	Password    string  `json:"password"`
	DisplayName *string `json:"display_name"`
	RoleID      string  `json:"role_id"`
	Status      string  `json:"status"`
}

const userSelect = `SELECT u.id,u.username,u.email,u.display_name,u.role_id,r.name,u.status,
	u.failed_login_attempts,u.locked_until,u.last_login_at,u.password_changed_at,u.created_at,u.updated_at
	FROM users u JOIN roles r ON r.id=u.role_id`

func normalizeUserInput(in *userInput, requirePassword bool) error {
	in.Username = strings.TrimSpace(in.Username)
	in.RoleID = strings.TrimSpace(in.RoleID)
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if in.Username == "" || len(in.Username) > 50 {
		return errors.New("กรุณาระบุชื่อผู้ใช้และชื่อต้องมีความยาวไม่เกิน 50 ตัวอักษร")
	}
	if _, err := uuid.Parse(in.RoleID); err != nil {
		return errors.New("รหัสบทบาทไม่ถูกต้อง")
	}
	if in.Status != "ACTIVE" && in.Status != "DISABLED" && in.Status != "LOCKED" {
		return errors.New("สถานะผู้ใช้ต้องเป็น ACTIVE, DISABLED หรือ LOCKED เท่านั้น")
	}
	if requirePassword && in.Password == "" {
		return errors.New("กรุณาระบุรหัสผ่าน")
	}
	if in.Password != "" && len(in.Password) < 8 {
		return errors.New("รหัสผ่านต้องมีความยาวอย่างน้อย 8 ตัวอักษร")
	}
	if in.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*in.Email))
		if email == "" {
			in.Email = nil
		} else if len(email) > 255 {
			return errors.New("อีเมลต้องมีความยาวไม่เกิน 255 ตัวอักษร")
		} else {
			in.Email = &email
		}
	}
	if in.DisplayName != nil {
		displayName := strings.TrimSpace(*in.DisplayName)
		if displayName == "" {
			in.DisplayName = nil
		} else if len(displayName) > 100 {
			return errors.New("ชื่อที่แสดงต้องมีความยาวไม่เกิน 100 ตัวอักษร")
		} else {
			in.DisplayName = &displayName
		}
	}
	return nil
}

func userDBError(c *fiber.Ctx, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23505":
			return c.Status(409).JSON(fiber.Map{"error": "ชื่อผู้ใช้หรืออีเมลนี้ถูกใช้งานแล้ว"})
		case "23503":
			return c.Status(400).JSON(fiber.Map{"error": "ไม่พบบทบาทที่ระบุ"})
		}
	}
	return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถดำเนินการกับข้อมูลผู้ใช้ได้"})
}

func userMap(id, username string, email, displayName sql.NullString, roleID, role, status string,
	failedAttempts int, lockedUntil, lastLogin, passwordChanged sql.NullTime, created, updated time.Time,
) fiber.Map {
	return fiber.Map{
		"id": id, "username": username, "email": nullString(email), "display_name": nullString(displayName),
		"role_id": roleID, "role": role, "status": status, "failed_login_attempts": failedAttempts,
		"locked_until": nullableTime(lockedUntil), "last_login_at": nullableTime(lastLogin),
		"password_changed_at": nullableTime(passwordChanged), "created_at": created, "updated_at": updated,
	}
}

func nullableTime(value sql.NullTime) interface{} {
	if value.Valid {
		return value.Time
	}
	return nil
}

func scanUser(scanner interface{ Scan(...interface{}) error }) (fiber.Map, error) {
	var id, username, roleID, role, status string
	var email, displayName sql.NullString
	var failedAttempts int
	var lockedUntil, lastLogin, passwordChanged sql.NullTime
	var created, updated time.Time
	err := scanner.Scan(&id, &username, &email, &displayName, &roleID, &role, &status, &failedAttempts,
		&lockedUntil, &lastLogin, &passwordChanged, &created, &updated)
	if err != nil {
		return nil, err
	}
	return userMap(id, username, email, displayName, roleID, role, status, failedAttempts,
		lockedUntil, lastLogin, passwordChanged, created, updated), nil
}

func ListUsers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rows, err := db.Query(userSelect + ` ORDER BY u.username`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการผู้ใช้ได้"})
		}
		defer rows.Close()
		users := make([]fiber.Map, 0)
		for rows.Next() {
			user, err := scanUser(rows)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการผู้ใช้ได้"})
			}
			users = append(users, user)
		}
		if err := rows.Err(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการผู้ใช้ได้"})
		}
		return c.JSON(fiber.Map{"users": users})
	}
}

func GetUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		user, err := scanUser(db.QueryRow(userSelect+` WHERE u.id=$1`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบผู้ใช้"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านข้อมูลผู้ใช้ได้"})
		}
		return c.JSON(user)
	}
}

func CreateManagedUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in userInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		if err := normalizeUserInput(&in, true); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถเข้ารหัสรหัสผ่านได้"})
		}
		var id string
		err = db.QueryRow(`INSERT INTO users(username,email,password_hash,display_name,role_id,status)
			VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, in.Username, in.Email, string(hash), in.DisplayName, in.RoleID, in.Status).Scan(&id)
		if err != nil {
			return userDBError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"message": "สร้างผู้ใช้สำเร็จ", "id": id})
	}
}

func UpdateUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var in userInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		if err := normalizeUserInput(&in, false); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}

		var result sql.Result
		if in.Password == "" {
			result, err = db.Exec(`UPDATE users SET username=$1,email=$2,display_name=$3,role_id=$4,status=$5,updated_at=NOW()
				WHERE id=$6`, in.Username, in.Email, in.DisplayName, in.RoleID, in.Status, id)
		} else {
			var hash []byte
			hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
			if err == nil {
				result, err = db.Exec(`UPDATE users SET username=$1,email=$2,password_hash=$3,display_name=$4,role_id=$5,
					status=$6,password_changed_at=NOW(),updated_at=NOW() WHERE id=$7`, in.Username, in.Email, string(hash),
					in.DisplayName, in.RoleID, in.Status, id)
			}
		}
		if err != nil {
			return userDBError(c, err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบผู้ใช้"})
		}
		return c.JSON(fiber.Map{"message": "แก้ไขผู้ใช้สำเร็จ"})
	}
}

func DeleteUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		result, err := db.Exec(`DELETE FROM users WHERE id=$1`, id)
		if err != nil {
			return dbError(c, err, "ผู้ใช้นี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบผู้ใช้"})
		}
		return c.SendStatus(204)
	}
}
