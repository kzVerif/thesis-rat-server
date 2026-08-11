package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type roleInput struct {
	Name          string    `json:"name"`
	Description   *string   `json:"description"`
	PermissionIDs *[]string `json:"permission_ids"`
}

type permissionInput struct {
	Code        string  `json:"code"`
	Description *string `json:"description"`
}

const roleSelect = `SELECT r.id,r.name,r.description,r.created_at,r.updated_at,
	COALESCE(array_agg(p.id::text ORDER BY p.code) FILTER (WHERE p.id IS NOT NULL),'{}'),
	COALESCE(array_agg(p.code ORDER BY p.code) FILTER (WHERE p.id IS NOT NULL),'{}')
	FROM roles r LEFT JOIN role_permissions rp ON rp.role_id=r.id
	LEFT JOIN permissions p ON p.id=rp.permission_id`

func routeID(c *fiber.Ctx) (string, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return "", c.Status(400).JSON(fiber.Map{"error": "รหัสไอดีไม่ถูกต้อง"})
	}
	return id, nil
}

func dbError(c *fiber.Ctx, err error, duplicate string) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pqErr.Code == "23505" {
			return c.Status(409).JSON(fiber.Map{"error": duplicate})
		}
		if pqErr.Code == "23503" {
			return c.Status(409).JSON(fiber.Map{"error": "ทรัพยากรกำลังถูกใช้งานอยู่"})
		}
	}
	return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถดำเนินการกับฐานข้อมูลได้"})
}

func nullString(v sql.NullString) interface{} {
	if v.Valid {
		return v.String
	}
	return nil
}

func roleMap(id, name string, description sql.NullString, created, updated time.Time, ids, codes pq.StringArray) fiber.Map {
	return fiber.Map{"id": id, "name": name, "description": nullString(description), "created_at": created,
		"updated_at": updated, "permission_ids": []string(ids), "permissions": []string(codes)}
}

func isProtectedRole(name string) bool {
	return name == "VIEWER" || name == "ADMINISTRATOR"
}

func ListRoles(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rows, err := db.Query(roleSelect + ` GROUP BY r.id ORDER BY r.name`)
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		defer rows.Close()
		roles := make([]fiber.Map, 0)
		for rows.Next() {
			var id, name string
			var d sql.NullString
			var created, updated time.Time
			var ids, codes pq.StringArray
			if err := rows.Scan(&id, &name, &d, &created, &updated, &ids, &codes); err != nil {
				return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
			}
			roles = append(roles, roleMap(id, name, d, created, updated, ids, codes))
		}
		if err := rows.Err(); err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		return c.JSON(fiber.Map{"roles": roles})
	}
}

func GetRole(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var name string
		var d sql.NullString
		var created, updated time.Time
		var ids, codes pq.StringArray
		err = db.QueryRow(roleSelect+` WHERE r.id=$1 GROUP BY r.id`, id).Scan(&id, &name, &d, &created, &updated, &ids, &codes)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบบทบาท"})
		}
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		return c.JSON(roleMap(id, name, d, created, updated, ids, codes))
	}
}

func replacePermissions(tx *sql.Tx, roleID string, ids []string) error {
	unique := make(map[string]bool, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil {
			return errors.New("รหัสสิทธิ์ไม่ถูกต้อง")
		}
		unique[id] = true
	}
	if len(unique) > 0 {
		var count int
		if err := tx.QueryRow(`SELECT count(*) FROM permissions WHERE id=ANY($1::uuid[])`, pq.Array(ids)).Scan(&count); err != nil {
			return err
		}
		if count != len(unique) {
			return errors.New("ไม่พบสิทธิ์")
		}
	}
	if _, err := tx.Exec(`DELETE FROM role_permissions WHERE role_id=$1`, roleID); err != nil {
		return err
	}
	for id := range unique {
		if _, err := tx.Exec(`INSERT INTO role_permissions(role_id,permission_id) VALUES($1,$2)`, roleID, id); err != nil {
			return err
		}
	}
	return nil
}

func CreateRole(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in roleInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 50 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุชื่อและชื่อต้องมีความยาวไม่เกิน 50 ตัวอักษร"})
		}
		tx, err := db.Begin()
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		defer tx.Rollback()
		var id string
		if err = tx.QueryRow(`INSERT INTO roles(name,description) VALUES($1,$2) RETURNING id`, in.Name, in.Description).Scan(&id); err != nil {
			return dbError(c, err, "ชื่อบทบาทนี้มีอยู่แล้ว")
		}
		if in.PermissionIDs != nil {
			if err = replacePermissions(tx, id, *in.PermissionIDs); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}
		if err = tx.Commit(); err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		return c.Status(201).JSON(fiber.Map{"message": "สร้างบทบาทสำเร็จ", "id": id})
	}
}

func UpdateRole(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var in roleInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 50 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุชื่อและชื่อต้องมีความยาวไม่เกิน 50 ตัวอักษร"})
		}
		tx, err := db.Begin()
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		defer tx.Rollback()

		var currentName string
		err = tx.QueryRow(`SELECT name FROM roles WHERE id=$1 FOR UPDATE`, id).Scan(&currentName)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบบทบาท"})
		}
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		if isProtectedRole(currentName) && in.Name != currentName {
			return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถเปลี่ยนชื่อบทบาทที่ระบบป้องกันไว้ได้"})
		}

		result, err := tx.Exec(`UPDATE roles SET name=$1,description=$2,updated_at=NOW() WHERE id=$3`, in.Name, in.Description, id)
		if err != nil {
			return dbError(c, err, "ชื่อบทบาทนี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบบทบาท"})
		}
		if in.PermissionIDs != nil {
			if err = replacePermissions(tx, id, *in.PermissionIDs); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": err.Error()})
			}
		}
		if err = tx.Commit(); err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		return c.JSON(fiber.Map{"message": "แก้ไขบทบาทสำเร็จ"})
	}
}

func DeleteRole(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		tx, err := db.Begin()
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		defer tx.Rollback()

		var name string
		err = tx.QueryRow(`SELECT name FROM roles WHERE id=$1 FOR UPDATE`, id).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบบทบาท"})
		}
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		if isProtectedRole(name) {
			return c.Status(400).JSON(fiber.Map{"error": "ไม่สามารถลบบทบาทที่ระบบป้องกันไว้ได้"})
		}

		_, err = tx.Exec(`DELETE FROM roles WHERE id=$1`, id)
		if err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		if err = tx.Commit(); err != nil {
			return dbError(c, err, "บทบาทนี้มีอยู่แล้ว")
		}
		return c.SendStatus(204)
	}
}

func ListPermissions(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rows, err := db.Query(`SELECT id,code,description,created_at FROM permissions ORDER BY code`)
		if err != nil {
			return dbError(c, err, "สิทธิ์นี้มีอยู่แล้ว")
		}
		defer rows.Close()
		items := make([]fiber.Map, 0)
		for rows.Next() {
			var id, code string
			var d sql.NullString
			var created time.Time
			if err := rows.Scan(&id, &code, &d, &created); err != nil {
				return dbError(c, err, "สิทธิ์นี้มีอยู่แล้ว")
			}
			items = append(items, fiber.Map{"id": id, "code": code, "description": nullString(d), "created_at": created})
		}
		if err := rows.Err(); err != nil {
			return dbError(c, err, "สิทธิ์นี้มีอยู่แล้ว")
		}
		return c.JSON(fiber.Map{"permissions": items})
	}
}

func GetPermission(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var code string
		var d sql.NullString
		var created time.Time
		err = db.QueryRow(`SELECT code,description,created_at FROM permissions WHERE id=$1`, id).Scan(&code, &d, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบสิทธิ์"})
		}
		if err != nil {
			return dbError(c, err, "สิทธิ์นี้มีอยู่แล้ว")
		}
		return c.JSON(fiber.Map{"id": id, "code": code, "description": nullString(d), "created_at": created})
	}
}

func CreatePermission(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in permissionInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Code = strings.TrimSpace(in.Code)
		if in.Code == "" || len(in.Code) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุรหัสสิทธิ์และรหัสต้องมีความยาวไม่เกิน 100 ตัวอักษร"})
		}
		var id string
		err := db.QueryRow(`INSERT INTO permissions(code,description) VALUES($1,$2) RETURNING id`, in.Code, in.Description).Scan(&id)
		if err != nil {
			return dbError(c, err, "รหัสสิทธิ์นี้มีอยู่แล้ว")
		}
		return c.Status(201).JSON(fiber.Map{"message": "สร้างสิทธิ์สำเร็จ", "id": id})
	}
}

func UpdatePermission(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var in permissionInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Code = strings.TrimSpace(in.Code)
		if in.Code == "" || len(in.Code) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุรหัสสิทธิ์และรหัสต้องมีความยาวไม่เกิน 100 ตัวอักษร"})
		}
		result, err := db.Exec(`UPDATE permissions SET code=$1,description=$2 WHERE id=$3`, in.Code, in.Description, id)
		if err != nil {
			return dbError(c, err, "รหัสสิทธิ์นี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบสิทธิ์"})
		}
		return c.JSON(fiber.Map{"message": "แก้ไขสิทธิ์สำเร็จ"})
	}
}

func DeletePermission(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		result, err := db.Exec(`DELETE FROM permissions WHERE id=$1`, id)
		if err != nil {
			return dbError(c, err, "สิทธิ์นี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบสิทธิ์"})
		}
		return c.SendStatus(204)
	}
}
