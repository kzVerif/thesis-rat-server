package service

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const TokensManagePermission = "tokens.manage"

// Plaintext enrollment secrets are returned only by CreateToken.
type TokenRecord struct {
	ID              string     `json:"id"`
	CreatedBy       *string    `json:"created_by"`
	CreatorUsername *string    `json:"creator_username"`
	MaxUse          *int64     `json:"max_use"`
	UsedCount       int64      `json:"used_count"`
	ExpiresAt       *time.Time `json:"expires_at"`
	IsRevoked       bool       `json:"is_revoked"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

const tokenColumns = `id,created_by,max_use,used_count,expires_at,is_revoked,created_at,updated_at`
const tokenSelect = `SELECT t.id,t.created_by,t.max_use,t.used_count,t.expires_at,t.is_revoked,t.created_at,t.updated_at,u.username
	FROM tokens t LEFT JOIN users u ON u.id=t.created_by`

func scanToken(row interface{ Scan(...interface{}) error }, withUsername bool) (TokenRecord, error) {
	var item TokenRecord
	args := []interface{}{&item.ID, &item.CreatedBy, &item.MaxUse, &item.UsedCount, &item.ExpiresAt, &item.IsRevoked, &item.CreatedAt, &item.UpdatedAt}
	if withUsername {
		args = append(args, &item.CreatorUsername)
	}
	err := row.Scan(args...)
	return item, err
}

func decodeTokenJSON(body []byte, target interface{}) error {
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return errors.New("JSON object is required")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return errors.New("invalid JSON or unsupported field")
	}
	if err := d.Decode(new(interface{})); err != io.EOF {
		return errors.New("exactly one JSON object is required")
	}
	return nil
}

type tokenInput struct {
	MaxUse    json.RawMessage `json:"max_use"`
	ExpiresAt json.RawMessage `json:"expires_at"`
}

func (in tokenInput) values() (*int64, *time.Time, error) {
	var max *int64
	var expires *time.Time
	if len(in.MaxUse) > 0 {
		if err := json.Unmarshal(in.MaxUse, &max); err != nil || (max != nil && (*max < 1 || *max > 2147483647)) {
			return nil, nil, errors.New("max_use must be an integer between 1 and 2147483647, or null")
		}
	}
	if len(in.ExpiresAt) > 0 {
		if err := json.Unmarshal(in.ExpiresAt, &expires); err != nil || (expires != nil && !expires.After(time.Now())) {
			return nil, nil, errors.New("expires_at must be a future RFC3339 timestamp, or null")
		}
	}
	return max, expires, nil
}

func tokenError(c *fiber.Ctx) error {
	return c.Status(500).JSON(fiber.Map{"error": "unable to process enrollment token"})
}
func tokenID(c *fiber.Ctx) (string, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return "", errors.New("id must be a valid UUID")
	}
	return id.String(), nil
}

func ListTokens(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, err := positiveQueryInt(c, "page", 1, 0)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid page"})
		}
		limit, err := positiveQueryInt(c, "limit", 20, 100)
		if err != nil || int64(page-1) > int64(^uint64(0)>>1)/int64(limit) {
			return c.Status(400).JSON(fiber.Map{"error": "invalid pagination"})
		}
		tx, err := db.BeginTx(c.UserContext(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			return tokenError(c)
		}
		defer tx.Rollback()
		var total int64
		if err := tx.QueryRowContext(c.UserContext(), `SELECT COUNT(*) FROM tokens`).Scan(&total); err != nil {
			return tokenError(c)
		}
		rows, err := tx.QueryContext(c.UserContext(), tokenSelect+` ORDER BY t.created_at DESC,t.id DESC LIMIT $1 OFFSET $2`, limit, int64(page-1)*int64(limit))
		if err != nil {
			return tokenError(c)
		}
		defer rows.Close()
		items := make([]TokenRecord, 0)
		for rows.Next() {
			item, err := scanToken(rows, true)
			if err != nil {
				return tokenError(c)
			}
			items = append(items, item)
		}
		if rows.Err() != nil {
			return tokenError(c)
		}
		if err := rows.Close(); err != nil {
			return tokenError(c)
		}
		if err := tx.Commit(); err != nil {
			return tokenError(c)
		}
		pages := total / int64(limit)
		if total%int64(limit) != 0 {
			pages++
		}
		return c.JSON(fiber.Map{"tokens": items, "pagination": fiber.Map{"page": page, "limit": limit, "total": total, "total_pages": pages}})
	}
}

func GetToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := tokenID(c)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		item, err := scanToken(db.QueryRowContext(c.UserContext(), tokenSelect+` WHERE t.id=$1`, id), true)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "token not found"})
		}
		if err != nil {
			return tokenError(c)
		}
		return c.JSON(item)
	}
}

func CreateToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in tokenInput
		if err := decodeTokenJSON(c.Body(), &in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		max, expires, err := in.values()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		secret, err := randomToken()
		if err != nil {
			return tokenError(c)
		}
		user := signedInUser(c)
		item, err := scanToken(db.QueryRowContext(c.UserContext(), `INSERT INTO tokens(created_by,token_hash,max_use,expires_at)
			VALUES($1,$2,$3,$4) RETURNING `+tokenColumns, user.ID, hashToken(secret), max, expires), false)
		if err != nil {
			return tokenError(c)
		}
		item.CreatorUsername = &user.Username
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.Status(201).JSON(struct {
			TokenRecord
			Token string `json:"token"`
		}{item, secret})
	}
}

func UpdateToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := tokenID(c)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		var in tokenInput
		if err := decodeTokenJSON(c.Body(), &in); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if len(in.MaxUse) == 0 && len(in.ExpiresAt) == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "no editable fields"})
		}
		max, expires, err := in.values()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		tx, err := db.BeginTx(c.UserContext(), nil)
		if err != nil {
			return tokenError(c)
		}
		defer tx.Rollback()
		var used int64
		err = tx.QueryRowContext(c.UserContext(), `SELECT used_count FROM tokens WHERE id=$1 FOR UPDATE`, id).Scan(&used)
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "token not found"})
		}
		if err != nil {
			return tokenError(c)
		}
		if max != nil && *max < used {
			return c.Status(409).JSON(fiber.Map{"error": "max_use cannot be less than used_count"})
		}
		_, err = tx.ExecContext(c.UserContext(), `UPDATE tokens SET max_use=CASE WHEN $2 THEN $3::integer ELSE max_use END,
			expires_at=CASE WHEN $4 THEN $5::timestamptz ELSE expires_at END,updated_at=clock_timestamp() WHERE id=$1`,
			id, len(in.MaxUse) > 0, max, len(in.ExpiresAt) > 0, expires)
		if err != nil {
			return tokenError(c)
		}
		if err := tx.Commit(); err != nil {
			return tokenError(c)
		}
		return c.JSON(fiber.Map{"message": "updated"})
	}
}

// DELETE terminates enrollment while preserving creator and usage history.
func RevokeToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := tokenID(c)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		res, err := db.ExecContext(c.UserContext(), `UPDATE tokens SET is_revoked=TRUE,
			updated_at=CASE WHEN is_revoked THEN updated_at ELSE clock_timestamp() END WHERE id=$1`, id)
		if err != nil {
			return tokenError(c)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return tokenError(c)
		}
		if n == 0 {
			return c.Status(404).JSON(fiber.Map{"error": "token not found"})
		}
		return c.JSON(fiber.Map{"message": "revoked"})
	}
}

const usableTokenCondition = `token_hash=$1 AND NOT is_revoked
	AND (expires_at IS NULL OR expires_at>clock_timestamp())
	AND (max_use IS NULL OR used_count<max_use) AND used_count<2147483647`

// Optional preflight; only RegisterAgent consumes a use.
func ValidateToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var in struct {
			Token string `json:"token"`
		}
		if err := decodeTokenJSON(c.Body(), &in); err != nil || strings.TrimSpace(in.Token) == "" || len(in.Token) > 512 {
			return c.Status(400).JSON(fiber.Map{"error": "token is required (maximum 512 bytes)"})
		}
		var valid bool
		err := db.QueryRowContext(c.UserContext(), `SELECT EXISTS(SELECT 1 FROM tokens WHERE `+usableTokenCondition+`)`, hashToken(in.Token)).Scan(&valid)
		if err != nil {
			return tokenError(c)
		}
		c.Set(fiber.HeaderCacheControl, "no-store")
		if !valid {
			return c.Status(403).JSON(fiber.Map{"valid": false, "error": "token is invalid, expired, revoked, or exhausted"})
		}
		return c.JSON(fiber.Map{"valid": true})
	}
}
