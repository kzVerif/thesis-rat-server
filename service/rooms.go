package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

const RoomsManagePermission = "rooms.manage"

type roomInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func RequirePermission(db *sql.DB, code string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		u, ok := c.Locals(authUserLocal).(*authUser)
		if !ok || u == nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "ไม่ได้เข้าสู่ระบบ"})
		}

		var allowed bool
		err := db.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM role_permissions rp
			JOIN permissions p ON p.id=rp.permission_id
			WHERE rp.role_id=$1 AND p.code=$2
		)`, u.RoleID, code).Scan(&allowed)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "ไม่สามารถตรวจสอบสิทธิ์ได้"})
		}
		if !allowed {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "ไม่มีสิทธิ์ดำเนินการนี้"})
		}
		return c.Next()
	}
}

func roomMap(id, name string, description sql.NullString, created, updated time.Time, agentCount, onlineAgentCount, offlineAgentCount int64) fiber.Map {
	return fiber.Map{
		"id": id, "name": name, "description": nullString(description),
		"created_at": created, "updated_at": updated,
		"agent_count": agentCount, "online_agent_count": onlineAgentCount, "offline_agent_count": offlineAgentCount,
	}
}

func ListRooms(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rows, err := db.Query(`SELECT r.id,r.name,r.description,r.created_at,r.updated_at,COUNT(a.id),
			COUNT(a.id) FILTER (WHERE a.status='ONLINE'),
			COUNT(a.id) FILTER (WHERE a.status='OFFLINE')
			FROM rooms r LEFT JOIN agents a ON a.room_id=r.id
			GROUP BY r.id ORDER BY r.name`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการห้องได้"})
		}
		defer rows.Close()

		rooms := make([]fiber.Map, 0)
		for rows.Next() {
			var id, name string
			var description sql.NullString
			var created, updated time.Time
			var agentCount, onlineAgentCount, offlineAgentCount int64
			if err := rows.Scan(&id, &name, &description, &created, &updated, &agentCount, &onlineAgentCount, &offlineAgentCount); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการห้องได้"})
			}
			rooms = append(rooms, roomMap(id, name, description, created, updated, agentCount, onlineAgentCount, offlineAgentCount))
		}
		if err := rows.Err(); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการห้องได้"})
		}
		return c.JSON(fiber.Map{"rooms": rooms})
	}
}

func GetRoom(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}

		var name string
		var description sql.NullString
		var created, updated time.Time
		var agentCount, onlineAgentCount, offlineAgentCount int64
		err = db.QueryRow(`SELECT r.name,r.description,r.created_at,r.updated_at,COUNT(a.id),
			COUNT(a.id) FILTER (WHERE a.status='ONLINE'),
			COUNT(a.id) FILTER (WHERE a.status='OFFLINE')
			FROM rooms r LEFT JOIN agents a ON a.room_id=r.id
			WHERE r.id=$1 GROUP BY r.id`, id).
			Scan(&name, &description, &created, &updated, &agentCount, &onlineAgentCount, &offlineAgentCount)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบห้อง"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านข้อมูลห้องได้"})
		}
		return c.JSON(roomMap(id, name, description, created, updated, agentCount, onlineAgentCount, offlineAgentCount))
	}
}

func CreateRoom(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in roomInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุชื่อห้องและชื่อต้องมีความยาวไม่เกิน 100 ตัวอักษร"})
		}

		var id string
		err := db.QueryRow(`INSERT INTO rooms(name,description) VALUES($1,$2) RETURNING id`, in.Name, in.Description).Scan(&id)
		if err != nil {
			return dbError(c, err, "ชื่อห้องนี้มีอยู่แล้ว")
		}
		return c.Status(201).JSON(fiber.Map{"message": "สร้างห้องสำเร็จ", "id": id})
	}
}

func UpdateRoom(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var in roomInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 100 {
			return c.Status(400).JSON(fiber.Map{"error": "กรุณาระบุชื่อห้องและชื่อต้องมีความยาวไม่เกิน 100 ตัวอักษร"})
		}

		result, err := db.Exec(`UPDATE rooms SET name=$1,description=$2,updated_at=NOW() WHERE id=$3`, in.Name, in.Description, id)
		if err != nil {
			return dbError(c, err, "ชื่อห้องนี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบห้อง"})
		}
		return c.JSON(fiber.Map{"message": "แก้ไขห้องสำเร็จ"})
	}
}

func DeleteRoom(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		result, err := db.Exec(`DELETE FROM rooms WHERE id=$1`, id)
		if err != nil {
			return dbError(c, err, "ห้องนี้มีอยู่แล้ว")
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบห้อง"})
		}
		return c.SendStatus(204)
	}
}
