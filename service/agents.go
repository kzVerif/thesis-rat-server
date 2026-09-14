package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	AgentsManagePermission = "agents.manage"
	AgentsReadPermission   = "agents.read"
	AgentsEditPermission   = "agents.edit"
	AgentsDeletePermission = "agents.delete"
)

var macAddressPattern = regexp.MustCompile(`(?i)^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

type agentInput struct {
	RoomID     *string         `json:"room_id"`
	Hostname   string          `json:"hostname"`
	OSInfo     json.RawMessage `json:"os_info"`
	MACAddress *string         `json:"mac_address"`
	IPAddress  *string         `json:"ip_address"`
	Status     string          `json:"status"`
	LastSeen   *time.Time      `json:"last_seen"`
	EnrolledAt *time.Time      `json:"enrolled_at"`
}

const agentSelect = `SELECT a.id,a.room_id,a.hostname,a.os_info,a.mac_address,
	COALESCE(host(a.ip_address),''),a.status,a.last_seen,a.enrolled_at,a.created_at,a.updated_at
	FROM agents a`

func normalizeAgentInput(in *agentInput) error {
	in.Hostname = strings.TrimSpace(in.Hostname)
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	if in.Hostname == "" || len(in.Hostname) > 255 {
		return errors.New("กรุณาระบุ hostname และต้องมีความยาวไม่เกิน 255 ตัวอักษร")
	}
	if in.Status == "" {
		in.Status = "OFFLINE"
	}
	if in.Status != "ONLINE" && in.Status != "OFFLINE" && in.Status != "WARNING" && in.Status != "DISABLED" {
		return errors.New("status ต้องเป็น ONLINE, OFFLINE, WARNING หรือ DISABLED")
	}
	if in.RoomID != nil {
		value := strings.TrimSpace(*in.RoomID)
		if value == "" {
			in.RoomID = nil
		} else if _, err := uuid.Parse(value); err != nil {
			return errors.New("room_id ไม่ถูกต้อง")
		} else {
			in.RoomID = &value
		}
	}
	if len(in.OSInfo) != 0 && string(in.OSInfo) != "null" && !json.Valid(in.OSInfo) {
		return errors.New("ข้อมูล JSON ใน os_info ไม่ถูกต้อง")
	}
	if in.MACAddress != nil {
		value := strings.ToLower(strings.TrimSpace(*in.MACAddress))
		if value == "" {
			in.MACAddress = nil
		} else if !macAddressPattern.MatchString(value) {
			return errors.New("mac_address ต้องอยู่ในรูปแบบ 00:11:22:33:44:55")
		} else {
			in.MACAddress = &value
		}
	}
	if in.IPAddress != nil {
		value := strings.TrimSpace(*in.IPAddress)
		if value == "" {
			in.IPAddress = nil
		} else if net.ParseIP(value) == nil {
			return errors.New("ip_address ไม่ถูกต้อง")
		} else {
			in.IPAddress = &value
		}
	}
	return nil
}

func osInfoValue(raw json.RawMessage) interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}

func scanAgent(scanner interface{ Scan(...interface{}) error }) (fiber.Map, error) {
	var id, hostname, ipAddress, status string
	var roomID, macAddress sql.NullString
	var osInfo []byte
	var lastSeen, enrolledAt sql.NullTime
	var createdAt, updatedAt time.Time
	if err := scanner.Scan(&id, &roomID, &hostname, &osInfo, &macAddress, &ipAddress, &status,
		&lastSeen, &enrolledAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var decodedOSInfo interface{}
	if len(osInfo) > 0 {
		_ = json.Unmarshal(osInfo, &decodedOSInfo)
	}
	return fiber.Map{
		"id": id, "room_id": nullString(roomID), "hostname": hostname, "os_info": decodedOSInfo,
		"mac_address": nullString(macAddress), "ip_address": nullableString(ipAddress), "status": status,
		"last_seen": nullableTime(lastSeen), "enrolled_at": nullableTime(enrolledAt),
		"created_at": createdAt, "updated_at": updatedAt,
	}, nil
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func agentDBError(c *fiber.Ctx, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return c.Status(409).JSON(fiber.Map{"error": "agent MAC address is already registered"})
	}
	if errors.As(err, &pqErr) && pqErr.Code == "23503" {
		return c.Status(400).JSON(fiber.Map{"error": "ไม่พบ room_id ที่ระบุ"})
	}
	return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถดำเนินการกับข้อมูล Agent ได้"})
}

func ListAgents(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, err := positiveQueryInt(c, "page", 1, 0)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "page ต้องเป็นจำนวนเต็มที่มากกว่า 0"})
		}
		limit, err := positiveQueryInt(c, "limit", 20, 100)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "limit ต้องเป็นจำนวนเต็มตั้งแต่ 1 ถึง 100"})
		}
		if int64(page-1) > int64(^uint64(0)>>1)/int64(limit) {
			return c.Status(400).JSON(fiber.Map{"error": "ค่า page มีขนาดใหญ่เกินไป"})
		}
		offset := int64(page-1) * int64(limit)

		var total int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&total); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ Agent ได้"})
		}
		rows, err := db.Query(agentSelect+` ORDER BY a.hostname,a.id LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ Agent ได้"})
		}
		defer rows.Close()
		items := make([]fiber.Map, 0)
		for rows.Next() {
			item, err := scanAgent(rows)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ Agent ได้"})
			}
			items = append(items, item)
		}
		if rows.Err() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ Agent ได้"})
		}
		totalPages := int64(0)
		if total > 0 {
			totalPages = (total + int64(limit) - 1) / int64(limit)
		}
		return c.JSON(fiber.Map{
			"agents": items,
			"pagination": fiber.Map{
				"page": page, "limit": limit, "total": total, "total_pages": totalPages,
			},
		})
	}
}

func positiveQueryInt(c *fiber.Ctx, name string, defaultValue, maximum int) (int, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 || (maximum > 0 && value > maximum) {
		return 0, errors.New("ค่าต้องเป็นจำนวนเต็มที่มากกว่า 0")
	}
	return value, nil
}

func GetAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		item, err := scanAgent(db.QueryRow(agentSelect+` WHERE a.id=$1`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบ Agent"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านข้อมูล Agent ได้"})
		}
		return c.JSON(item)
	}
}

func CreateAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in agentInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "ข้อมูลที่ส่งมาไม่ถูกต้อง"})
		}
		if err := normalizeAgentInput(&in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		var id string
		err := db.QueryRow(`INSERT INTO agents(room_id,hostname,os_info,mac_address,ip_address,status,last_seen,enrolled_at)
			VALUES($1,$2,$3::jsonb,$4,NULLIF($5,'')::inet,$6,$7,$8) RETURNING id`, in.RoomID, in.Hostname,
			osInfoValue(in.OSInfo), in.MACAddress, in.IPAddress, in.Status, in.LastSeen, in.EnrolledAt).Scan(&id)
		if err != nil {
			return agentDBError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"message": "สร้าง Agent สำเร็จ", "id": id})
	}
}

func UpdateAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		var in agentInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "ข้อมูลที่ส่งมาไม่ถูกต้อง"})
		}
		if err := normalizeAgentInput(&in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		result, err := db.Exec(`UPDATE agents SET room_id=$1,hostname=$2,os_info=$3::jsonb,mac_address=$4,
			ip_address=NULLIF($5,'')::inet,status=$6,last_seen=$7,enrolled_at=$8,updated_at=NOW() WHERE id=$9`,
			in.RoomID, in.Hostname, osInfoValue(in.OSInfo), in.MACAddress, in.IPAddress, in.Status, in.LastSeen, in.EnrolledAt, id)
		if err != nil {
			return agentDBError(c, err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบ Agent"})
		}
		return c.JSON(fiber.Map{"message": "แก้ไข Agent สำเร็จ"})
	}
}

func DeleteAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := routeID(c)
		if err != nil {
			return err
		}
		result, err := db.Exec(`DELETE FROM agents WHERE id=$1`, id)
		if err != nil {
			return agentDBError(c, err)
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบ Agent"})
		}
		return c.SendStatus(204)
	}
}
