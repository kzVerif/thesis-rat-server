package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// RegisterAgent uses an enrollment token, not a human user's session.
// Neither agents nor logs store the token or its identifier.
func RegisterAgent(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in struct {
			Token      string          `json:"token"`
			Hostname   string          `json:"hostname"`
			MACAddress string          `json:"mac_address"`
			OSInfo     json.RawMessage `json:"os_info"`
			RoomID     *string         `json:"room_id"`
			AgentID    *string         `json:"agent_id"`
			PublicKey  string          `json:"public_key"`
		}
		if err := decodeTokenJSON(c.Body(), &in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if strings.TrimSpace(in.Token) == "" || len(in.Token) > 512 {
			return c.Status(400).JSON(fiber.Map{"error": "token is required (maximum 512 bytes)"})
		}
		if strings.TrimSpace(in.MACAddress) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "mac_address is required"})
		}
		if in.AgentID == nil || strings.TrimSpace(*in.AgentID) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "agent_id is required"})
		}
		if strings.TrimSpace(in.PublicKey) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "public_key is required"})
		}
		agentID := strings.TrimSpace(*in.AgentID)
		publicKey := in.PublicKey
		if _, err := uuid.Parse(agentID); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "agent_id must be a valid UUID"})
		}
		ip := c.IP()
		agent := agentInput{Hostname: in.Hostname, MACAddress: &in.MACAddress, OSInfo: in.OSInfo, RoomID: in.RoomID, IPAddress: &ip}
		if err := normalizeAgentInput(&agent); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tx, err := db.BeginTx(c.UserContext(), nil)
		if err != nil {
			return tokenError(c)
		}
		defer tx.Rollback()
		// Conditional UPDATE serializes consumers. Rollback refunds failed inserts.
		var consumed int
		err = tx.QueryRowContext(c.UserContext(), `UPDATE tokens SET used_count=used_count+1,updated_at=clock_timestamp()
			WHERE `+usableTokenCondition+` RETURNING used_count`, hashToken(in.Token)).Scan(&consumed)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(403).JSON(fiber.Map{"error": "token is invalid, expired, revoked, or exhausted"})
		}
		if err != nil {
			return tokenError(c)
		}
		var id string
		err = tx.QueryRowContext(c.UserContext(), `INSERT INTO agents(id,room_id,hostname,os_info,mac_address,ip_address,status,public_key,enrolled_at)
			VALUES($1,$2,$3,$4::jsonb,$5,$6::inet,'OFFLINE',$7,clock_timestamp()) RETURNING id`,
			agentID, agent.RoomID, agent.Hostname, osInfoValue(agent.OSInfo), agent.MACAddress, ip, publicKey).Scan(&id)
		if err != nil {
			var pgErr *pq.Error
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				if pgErr.Constraint == "agents_pkey" {
					return c.Status(409).JSON(fiber.Map{"error": "agent_id is already registered"})
				}
				return c.Status(409).JSON(fiber.Map{"error": "agent MAC address is already registered"})
			}
			return agentDBError(c, err)
		}
		if err := tx.Commit(); err != nil {
			return tokenError(c)
		}
		return c.Status(201).JSON(fiber.Map{"id": id, "message": "agent registered"})
	}
}
