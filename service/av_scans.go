package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const AVReadPermission = "av.read"

const avScanSelect = `SELECT id,agent_id,command_id,job_id,scan_type,started_at,finished_at,
	total_files_scanned,threats_found,threat_details,status,created_at FROM av_scan_results`

func scanAVResult(scanner interface{ Scan(...interface{}) error }) (fiber.Map, error) {
	var id, agentID, commandID, jobID, scanType, status string
	var started, finished sql.NullTime
	var totalFiles, threats int64
	var details []byte
	var created time.Time
	if err := scanner.Scan(&id, &agentID, &commandID, &jobID, &scanType, &started, &finished,
		&totalFiles, &threats, &details, &status, &created); err != nil {
		return nil, err
	}
	var threatDetails interface{}
	if len(details) > 0 {
		if err := json.Unmarshal(details, &threatDetails); err != nil {
			return nil, err
		}
	}
	return fiber.Map{
		"id": id, "agent_id": agentID, "command_id": commandID, "job_id": jobID,
		"scan_type": scanType, "started_at": nullableTime(started), "finished_at": nullableTime(finished),
		"total_files_scanned": totalFiles, "threats_found": threats, "threat_details": threatDetails,
		"status": status, "created_at": created,
	}, nil
}

func ListAVScanResults(db *sql.DB) fiber.Handler {
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
		where := " WHERE TRUE"
		args := make([]interface{}, 0)
		for _, field := range []string{"agent_id", "job_id", "command_id"} {
			if value := strings.TrimSpace(c.Query(field)); value != "" {
				id, err := uuid.Parse(value)
				if err != nil {
					return c.Status(400).JSON(fiber.Map{"error": field + " must be a valid UUID"})
				}
				args = append(args, id.String())
				where += fmt.Sprintf(" AND %s=$%d", field, len(args))
			}
		}
		if status := strings.ToUpper(strings.TrimSpace(c.Query("status"))); status != "" {
			switch status {
			case "PENDING", "RUNNING", "COMPLETED", "FAILED", "CANCELLED":
			default:
				return c.Status(400).JSON(fiber.Map{"error": "invalid status"})
			}
			args = append(args, status)
			where += fmt.Sprintf(" AND status=$%d", len(args))
		}
		var total int64
		if err := db.QueryRowContext(c.UserContext(), `SELECT COUNT(*) FROM av_scan_results`+where, args...).Scan(&total); err != nil {
			return avScanError(c)
		}
		query := avScanSelect + where + fmt.Sprintf(" ORDER BY created_at DESC,id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		args = append(args, limit, int64(page-1)*int64(limit))
		rows, err := db.QueryContext(c.UserContext(), query, args...)
		if err != nil {
			return avScanError(c)
		}
		defer rows.Close()
		items := make([]fiber.Map, 0)
		for rows.Next() {
			item, err := scanAVResult(rows)
			if err != nil {
				return avScanError(c)
			}
			items = append(items, item)
		}
		if rows.Err() != nil {
			return avScanError(c)
		}
		totalPages := total / int64(limit)
		if total%int64(limit) != 0 {
			totalPages++
		}
		return c.JSON(fiber.Map{"av_scan_results": items, "pagination": fiber.Map{
			"page": page, "limit": limit, "total": total, "total_pages": totalPages,
		}})
	}
}

func GetAVScanResult(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := uuid.Parse(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "id must be a valid UUID"})
		}
		item, err := scanAVResult(db.QueryRowContext(c.UserContext(), avScanSelect+` WHERE id=$1`, id.String()))
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "scan result not found"})
		}
		if err != nil {
			return avScanError(c)
		}
		return c.JSON(item)
	}
}

func avScanError(c *fiber.Ctx) error {
	return c.Status(500).JSON(fiber.Map{"error": "unable to read antivirus scan results"})
}
