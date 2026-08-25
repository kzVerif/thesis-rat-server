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
		return errors.New("hostname is required and must not exceed 255 characters")
	}
	if in.Status == "" {
		in.Status = "OFFLINE"
	}
	if in.Status != "ONLINE" && in.Status != "OFFLINE" && in.Status != "WARNING" && in.Status != "DISABLED" {
		return errors.New("status must be ONLINE, OFFLINE, WARNING, or DISABLED")
	}
	if in.RoomID != nil {
		value := strings.TrimSpace(*in.RoomID)
		if value == "" {
			in.RoomID = nil
		} else if _, err := uuid.Parse(value); err != nil {
			return errors.New("invalid room_id")
		} else {
			in.RoomID = &value
		}
	}
	if len(in.OSInfo) != 0 && string(in.OSInfo) != "null" && !json.Valid(in.OSInfo) {
		return errors.New("invalid os_info JSON")
	}
	if in.MACAddress != nil {
		value := strings.ToLower(strings.TrimSpace(*in.MACAddress))
		if value == "" {
			in.MACAddress = nil
		} else if !macAddressPattern.MatchString(value) {
			return errors.New("mac_address must use the format 00:11:22:33:44:55")
		} else {
			in.MACAddress = &value
		}
	}
	if in.IPAddress != nil {
		value := strings.TrimSpace(*in.IPAddress)
		if value == "" {
			in.IPAddress = nil
		} else if net.ParseIP(value) == nil {
			return errors.New("invalid ip_address")
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
	if errors.As(err, &pqErr) && pqErr.Code == "23503" {
		return c.Status(400).JSON(fiber.Map{"error": "room_id does not exist"})
	}
	return c.Status(500).JSON(fiber.Map{"error": "unable to process agent data"})
}

func ListAgents(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, err := positiveQueryInt(c, "page", 1, 0)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "page must be a positive integer"})
		}
		limit, err := positiveQueryInt(c, "limit", 20, 100)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "limit must be a positive integer not exceeding 100"})
		}
		if int64(page-1) > int64(^uint64(0)>>1)/int64(limit) {
			return c.Status(400).JSON(fiber.Map{"error": "page is too large"})
		}
		offset := int64(page-1) * int64(limit)

		var total int64
		if err := db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&total); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to list agents"})
		}
		rows, err := db.Query(agentSelect+` ORDER BY a.hostname,a.id LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to list agents"})
		}
		defer rows.Close()
		items := make([]fiber.Map, 0)
		for rows.Next() {
			item, err := scanAgent(rows)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "unable to list agents"})
			}
			items = append(items, item)
		}
		if rows.Err() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to list agents"})
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
		return 0, errors.New("invalid positive integer")
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
			return c.Status(404).JSON(fiber.Map{"error": "agent not found"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read agent"})
		}
		return c.JSON(item)
	}
}

func CreateAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in agentInput
		if c.BodyParser(&in) != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
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
		return c.Status(201).JSON(fiber.Map{"message": "agent created", "id": id})
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
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
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
			return c.Status(404).JSON(fiber.Map{"error": "agent not found"})
		}
		return c.JSON(fiber.Map{"message": "agent updated"})
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
			return c.Status(404).JSON(fiber.Map{"error": "agent not found"})
		}
		return c.SendStatus(204)
	}
}
