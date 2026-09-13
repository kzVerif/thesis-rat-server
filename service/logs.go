package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const LogsReadPermission = "logs.read"

type auditEvent struct {
	UserID  string
	AgentID string
	Action  string
	IP      string
	Detail  fiber.Map
}

// AuditRequests records mutations and selected failures, excluding routine reads.
// It never reads request bodies, cookies, headers, or query strings.
func AuditRequests(db *sql.DB) fiber.Handler {
	return auditRequests(func(event auditEvent) error {
		detail, err := json.Marshal(event.Detail)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		// Subqueries preserve the event when a user/agent was deleted by this request.
		_, err = db.ExecContext(ctx, `INSERT INTO logs(user_id,action,target_agent_id,detail,ip_address)
			VALUES ((SELECT id FROM users WHERE id=NULLIF($1,'')::uuid),$2,
			(SELECT id FROM agents WHERE id=NULLIF($3,'')::uuid),$4::jsonb,NULLIF($5,'')::inet)`,
			event.UserID, event.Action, event.AgentID, string(detail), event.IP)
		return err
	})
}

func auditRequests(write func(auditEvent) error) fiber.Handler {
	gate := &auditFailureGate{}
	return func(c *fiber.Ctx) error {
		started := time.Now()
		err := c.Next()
		// Render errors once so the recorded status matches the actual response.
		if err != nil {
			if handlerErr := c.App().ErrorHandler(c, err); handlerErr != nil {
				_ = c.Status(500).SendString("Internal Server Error")
			}
		}
		route := c.Route().Path
		status := c.Response().StatusCode()
		if !shouldAudit(c.Method(), status) {
			return nil
		}
		event := auditEvent{Action: c.Method() + " " + route, IP: c.IP(), Detail: fiber.Map{
			"method": c.Method(), "route": route, "status_code": status,
			"success": status >= 200 && status < 400, "duration_ms": time.Since(started).Milliseconds(),
		}}
		if len(event.Action) > 100 {
			event.Action = "API_REQUEST"
		}
		if net.ParseIP(event.IP) == nil {
			event.IP = ""
		}
		if u, ok := c.Locals(authUserLocal).(*authUser); ok && u != nil {
			event.UserID = u.ID
			event.Detail["actor_id"] = u.ID
			event.Detail["actor_username"] = u.Username
		}
		// Successful changes are never sampled. Repeated failures are bounded
		// by actor, IP, method, route and status, independently of target IDs.
		if status >= 400 && !gate.allow(auditFailureKey{event.UserID, event.IP, event.Action, status}, time.Now()) {
			return nil
		}
		for _, key := range []string{"id", "jobId", "filename"} {
			if value := c.Params(key); value != "" && len(value) <= 255 {
				event.Detail[key] = value
			}
		}
		// Decode only identifiers from successful responses; never copy whole responses.
		var result struct {
			ID      string `json:"id"`
			UserID  string `json:"user_id"`
			AgentID string `json:"agent_id"`
			File    struct {
				ID string `json:"id"`
			} `json:"file"`
		}
		if status >= 200 && status < 300 && strings.HasPrefix(string(c.Response().Header.ContentType()), "application/json") {
			_ = json.Unmarshal(c.Response().Body(), &result)
		}
		for key, value := range map[string]string{"resource_id": result.ID, "user_id": result.UserID, "file_id": result.File.ID} {
			if id, parseErr := uuid.Parse(value); parseErr == nil {
				event.Detail[key] = id.String()
			}
		}
		agentID := result.AgentID
		if strings.HasPrefix(route, "/api/agents") {
			agentID = c.Params("id")
			if agentID == "" {
				agentID = result.ID
			}
		}
		if id, parseErr := uuid.Parse(agentID); parseErr == nil {
			event.AgentID = id.String()
			event.Detail["target_agent_id"] = event.AgentID
		}
		if writeErr := write(event); writeErr != nil {
			// The business operation may already have committed. Do not tell clients to retry it.
			log.Printf("audit log write failed: action=%q status=%d error=%v", event.Action, status, writeErr)
		}
		return nil
	}
}

func shouldAudit(method string, status int) bool {
	if status == 401 || status == 403 || status == 429 || status >= 500 {
		return true
	}
	switch method {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		return true
	}
	return false
}

type auditFailureKey struct {
	user, ip, action string
	status           int
}

// A fixed one-minute window also bounds memory under many distinct source IPs.
// At capacity, new failure keys are omitted until the next window.
type auditFailureGate struct {
	mu     sync.Mutex
	window time.Time
	seen   map[auditFailureKey]struct{}
}

func (g *auditFailureGate) allow(key auditFailureKey, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.seen == nil || now.Sub(g.window) >= time.Minute {
		g.window = now
		g.seen = make(map[auditFailureKey]struct{})
	}
	if _, exists := g.seen[key]; exists {
		return false
	}
	if len(g.seen) >= 4096 {
		return false
	}
	g.seen[key] = struct{}{}
	return true
}

const logFrom = ` FROM logs l LEFT JOIN users u ON u.id=l.user_id LEFT JOIN agents a ON a.id=l.target_agent_id`
const logSelect = `SELECT l.id,l.user_id,u.username,u.display_name,l.action,l.target_agent_id,a.hostname,l.detail,host(l.ip_address),l.created_at` + logFrom

func scanLog(scanner interface{ Scan(...interface{}) error }) (fiber.Map, error) {
	var id, action string
	var userID, username, displayName, agentID, hostname, ip sql.NullString
	var detail json.RawMessage
	var created time.Time
	if err := scanner.Scan(&id, &userID, &username, &displayName, &action, &agentID, &hostname, &detail, &ip, &created); err != nil {
		return nil, err
	}
	return fiber.Map{"id": id, "user_id": nullString(userID), "username": nullString(username),
		"display_name": nullString(displayName), "action": action, "target_agent_id": nullString(agentID),
		"agent_hostname": nullString(hostname), "detail": detail, "ip_address": nullString(ip), "created_at": created}, nil
}

func logFilters(c *fiber.Ctx) (string, []interface{}, error) {
	where := " WHERE TRUE"
	args := make([]interface{}, 0)
	add := func(column, operator string, value interface{}) {
		args = append(args, value)
		where += fmt.Sprintf(" AND %s%s$%d", column, operator, len(args))
	}
	for _, field := range []string{"user_id", "target_agent_id"} {
		if value := strings.TrimSpace(c.Query(field)); value != "" {
			id, err := uuid.Parse(value)
			if err != nil {
				return "", nil, fmt.Errorf("%s must be a valid UUID", field)
			}
			add("l."+field, "=", id.String())
		}
	}
	if action := strings.TrimSpace(c.Query("action")); action != "" {
		if len(action) > 100 {
			return "", nil, errors.New("action must not exceed 100 bytes")
		}
		add("l.action", "=", action)
	}
	var from, to time.Time
	for _, field := range []string{"from", "to"} {
		if value := strings.TrimSpace(c.Query(field)); value != "" {
			t, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return "", nil, fmt.Errorf("%s must be an RFC3339 timestamp", field)
			}
			if field == "from" {
				from = t
				add("l.created_at", ">=", t)
			} else {
				to = t
				add("l.created_at", "<=", t)
			}
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return "", nil, errors.New("from must not be after to")
	}
	return where, args, nil
}

func ListLogs(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, err := positiveQueryInt(c, "page", 1, 0)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "page must be a positive integer"})
		}
		limit, err := positiveQueryInt(c, "limit", 20, 100)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "limit must be between 1 and 100"})
		}
		if int64(page-1) > int64(^uint64(0)>>1)/int64(limit) {
			return c.Status(400).JSON(fiber.Map{"error": "page is too large"})
		}
		where, args, err := logFilters(c)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		// Count and page use one consistent snapshot while new audit events arrive.
		tx, err := db.BeginTx(c.UserContext(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			return logsError(c)
		}
		defer tx.Rollback()
		var total int64
		if err := tx.QueryRowContext(c.UserContext(), `SELECT COUNT(*) FROM logs l`+where, args...).Scan(&total); err != nil {
			return logsError(c)
		}
		query := logSelect + where + fmt.Sprintf(" ORDER BY l.created_at DESC,l.id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, int64(page-1)*int64(limit))
		rows, err := tx.QueryContext(c.UserContext(), query, args...)
		if err != nil {
			return logsError(c)
		}
		defer rows.Close()
		items := make([]fiber.Map, 0)
		for rows.Next() {
			item, err := scanLog(rows)
			if err != nil {
				return logsError(c)
			}
			items = append(items, item)
		}
		if rows.Err() != nil {
			return logsError(c)
		}
		if err := rows.Close(); err != nil {
			return logsError(c)
		}
		if err := tx.Commit(); err != nil {
			return logsError(c)
		}
		pages := total / int64(limit)
		if total%int64(limit) != 0 {
			pages++
		}
		return c.JSON(fiber.Map{"logs": items, "pagination": fiber.Map{"page": page, "limit": limit, "total": total, "total_pages": pages}})
	}
}

func GetLog(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "id must be a valid UUID"})
		}
		item, err := scanLog(db.QueryRowContext(c.UserContext(), logSelect+` WHERE l.id=$1`, id.String()))
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "log not found"})
		}
		if err != nil {
			return logsError(c)
		}
		return c.JSON(item)
	}
}

func logsError(c *fiber.Ctx) error {
	return c.Status(500).JSON(fiber.Map{"error": "unable to read logs"})
}
