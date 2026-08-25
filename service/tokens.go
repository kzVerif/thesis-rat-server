package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type TokenRecord struct {
	ID        string     `json:"id"`
	UserID    *string    `json:"user_id,omitempty"`
	AgentID   *string    `json:"agent_id,omitempty"`
	Token     string     `json:"token"`
	TokenType string     `json:"token_type"`
	MaxUse    *int       `json:"max_use,omitempty"`
	UsedCount int        `json:"used_count"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IsRevoked bool       `json:"is_revoked"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListTokens returns all tokens (admin permission expected)
func ListTokens(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		rows, err := db.Query(`SELECT id,user_id,agent_id,token,token_type,max_use,used_count,expires_at,is_revoked,created_at FROM tokens ORDER BY created_at DESC`)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ tokens ได้"})
		}
		defer rows.Close()
		out := make([]TokenRecord, 0)
		for rows.Next() {
			var t TokenRecord
			var userID, agentID sql.NullString
			var maxUse sql.NullInt64
			var expires sql.NullTime
			if err := rows.Scan(&t.ID, &userID, &agentID, &t.Token, &t.TokenType, &maxUse, &t.UsedCount, &expires, &t.IsRevoked, &t.CreatedAt); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอ่านรายการ tokens ได้"})
			}
			if userID.Valid {
				v := userID.String
				t.UserID = &v
			}
			if agentID.Valid {
				v := agentID.String
				t.AgentID = &v
			}
			if maxUse.Valid {
				v := int(maxUse.Int64)
				t.MaxUse = &v
			}
			if expires.Valid {
				t.ExpiresAt = &expires.Time
			}
			out = append(out, t)
		}
		return c.JSON(fiber.Map{"tokens": out})
	}
}

// CreateToken creates a new token and returns the plaintext token
func CreateToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &raw); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}

		var userID *string
		if v, ok := raw["user_id"]; ok {
			if string(v) != "null" {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					userID = &s
				}
			}
		}

		var agentID *string
		if v, ok := raw["agent_id"]; ok {
			if string(v) != "null" {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					agentID = &s
				}
			}
		}

		tokenType := "api_key"
		if v, ok := raw["token_type"]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil && s != "" {
				tokenType = s
			}
		}

		var maxUse *int
		if v, ok := raw["max_use"]; ok {
			if string(v) != "null" {
				// accept number or string
				var num json.Number
				if err := json.Unmarshal(v, &num); err == nil {
					if i64, err := num.Int64(); err == nil {
						i := int(i64)
						maxUse = &i
					}
				} else {
					var s string
					if err := json.Unmarshal(v, &s); err == nil {
						if i64, err := strconv.ParseInt(s, 10, 64); err == nil {
							i := int(i64)
							maxUse = &i
						}
					}
				}
			}
		}

		var expiresAt *time.Time
		if v, ok := raw["expires_at"]; ok {
			if string(v) != "null" {
				var s string
				if err := json.Unmarshal(v, &s); err == nil {
					// try RFC3339 first
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						expiresAt = &t
					} else if t, err := time.Parse("2006-01-02", s); err == nil {
						// date-only string, treat as midnight UTC
						tt := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
						expiresAt = &tt
					}
				}
			}
		}

		token, err := randomToken()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถสร้าง token ได้"})
		}

		var id string
		err = db.QueryRow(`INSERT INTO tokens (user_id,agent_id,token,token_type,max_use,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
			userID, agentID, token, tokenType, maxUse, expiresAt).Scan(&id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถบันทึก token ได้"})
		}
		return c.Status(201).JSON(fiber.Map{"id": id, "token": token})
	}
}

// UpdateToken updates editable fields of a token
func UpdateToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		if id == "" {
			return c.Status(400).JSON(fiber.Map{"error": "id parameter is required"})
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(c.Body(), &raw); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}

		sets := make([]string, 0)
		args := make([]interface{}, 0)
		idx := 1

		// max_use or maxUse
		vMaxUse, hasMaxUse := raw["max_use"]
		if !hasMaxUse {
			vMaxUse, hasMaxUse = raw["maxUse"]
		}
		if hasMaxUse {
			strVal := strings.TrimSpace(string(vMaxUse))
			if strVal == "null" || strVal == `""` || strVal == "" {
				sets = append(sets, fmt.Sprintf("max_use=$%d", idx))
				args = append(args, nil)
				idx++
			} else {
				// try as number first
				var num json.Number
				if err := json.Unmarshal(vMaxUse, &num); err == nil {
					i64, err := num.Int64()
					if err != nil {
						return c.Status(400).JSON(fiber.Map{"error": "max_use must be an integer"})
					}
					sets = append(sets, fmt.Sprintf("max_use=$%d", idx))
					args = append(args, i64)
					idx++
				} else {
					// try as string
					var s string
					if err := json.Unmarshal(vMaxUse, &s); err != nil {
						return c.Status(400).JSON(fiber.Map{"error": "max_use must be an integer or string representing integer"})
					}
					s = strings.TrimSpace(s)
					if s == "" || s == "null" {
						sets = append(sets, fmt.Sprintf("max_use=$%d", idx))
						args = append(args, nil)
						idx++
					} else {
						i, err := strconv.ParseInt(s, 10, 64)
						if err != nil {
							return c.Status(400).JSON(fiber.Map{"error": "max_use must be an integer"})
						}
						sets = append(sets, fmt.Sprintf("max_use=$%d", idx))
						args = append(args, i)
						idx++
					}
				}
			}
		}

		// expires_at or expiresAt
		vExpiresAt, hasExpiresAt := raw["expires_at"]
		if !hasExpiresAt {
			vExpiresAt, hasExpiresAt = raw["expiresAt"]
		}
		if hasExpiresAt {
			strVal := strings.TrimSpace(string(vExpiresAt))
			if strVal == "null" || strVal == `""` || strVal == "" {
				sets = append(sets, fmt.Sprintf("expires_at=$%d", idx))
				args = append(args, nil)
				idx++
			} else {
				var s string
				if err := json.Unmarshal(vExpiresAt, &s); err != nil {
					return c.Status(400).JSON(fiber.Map{"error": "expires_at must be date string or null"})
				}
				s = strings.TrimSpace(s)
				if s == "" || s == "null" {
					sets = append(sets, fmt.Sprintf("expires_at=$%d", idx))
					args = append(args, nil)
					idx++
				} else {
					if t, err := time.Parse(time.RFC3339, s); err == nil {
						sets = append(sets, fmt.Sprintf("expires_at=$%d", idx))
						args = append(args, t)
						idx++
					} else if t, err := time.Parse("2006-01-02", s); err == nil {
						tt := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
						sets = append(sets, fmt.Sprintf("expires_at=$%d", idx))
						args = append(args, tt)
						idx++
					} else {
						return c.Status(400).JSON(fiber.Map{"error": "expires_at must be RFC3339 or YYYY-MM-DD format"})
					}
				}
			}
		}

		// is_revoked or isRevoked
		vIsRevoked, hasIsRevoked := raw["is_revoked"]
		if !hasIsRevoked {
			vIsRevoked, hasIsRevoked = raw["isRevoked"]
		}
		if hasIsRevoked {
			var val *bool
			if err := json.Unmarshal(vIsRevoked, &val); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "is_revoked must be boolean or null"})
			}
			sets = append(sets, fmt.Sprintf("is_revoked=$%d", idx))
			args = append(args, val)
			idx++
		}

		if len(sets) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "ไม่มีฟิลด์ให้แก้ไข"})
		}

		sqlStr := fmt.Sprintf("UPDATE tokens SET %s WHERE id=$%d", strings.Join(sets, ", "), idx)
		args = append(args, id)

		res, err := db.Exec(sqlStr, args...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถอัพเดต token ได้", "detail": err.Error(), "sql": sqlStr, "args": fmt.Sprintf("%v", args)})
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบ token"})
		}
		return c.JSON(fiber.Map{"message": "updated"})
	}
}

// RevokeToken sets is_revoked = true
func RevokeToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Params("id")
		if id == "" {
			return c.Status(400).JSON(fiber.Map{"error": "id parameter is required"})
		}
		res, err := db.Exec(`DELETE FROM tokens WHERE id=$1`, id)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถลบ token ได้"})
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "ไม่พบ token"})
		}
		return c.JSON(fiber.Map{"message": "deleted"})
	}
}

// ValidateToken checks a token and increments used_count when applicable
func ValidateToken(db *sql.DB) fiber.Handler {
	type req struct {
		Token string `json:"token"`
	}
	return func(c *fiber.Ctx) error {
		var in req
		if err := c.BodyParser(&in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "รูปแบบข้อมูลไม่ถูกต้อง"})
		}
		var id string
		var maxUse sql.NullInt64
		var usedCount int
		var expires sql.NullTime
		var isRevoked bool
		err := db.QueryRow(`SELECT id,max_use,used_count,expires_at,is_revoked FROM tokens WHERE token=$1`, in.Token).Scan(&id, &maxUse, &usedCount, &expires, &isRevoked)
		if err != nil {
			if err == sql.ErrNoRows {
				return c.Status(404).JSON(fiber.Map{"valid": false, "error": "token not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": "ไม่สามารถตรวจสอบ token ได้"})
		}
		if isRevoked {
			return c.Status(403).JSON(fiber.Map{"valid": false, "error": "token revoked"})
		}
		if expires.Valid && expires.Time.Before(time.Now()) {
			return c.Status(403).JSON(fiber.Map{"valid": false, "error": "token expired"})
		}
		if maxUse.Valid && int(maxUse.Int64) <= usedCount {
			return c.Status(403).JSON(fiber.Map{"valid": false, "error": "token use limit exceeded"})
		}
		// increment used_count
		_, _ = db.Exec(`UPDATE tokens SET used_count=used_count+1 WHERE id=$1`, id)
		return c.JSON(fiber.Map{"valid": true})
	}
}
