package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var distributionListStatuses = map[string]bool{
	"IN_PROGRESS": true, "COMPLETED": true, "PARTIAL_FAILED": true, "FAILED": true,
}

type distributionFile struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type distributionTarget struct {
	Type   string  `json:"type"`
	RoomID *string `json:"room_id"`
	Label  string  `json:"label"`
}

type distributionSummary struct {
	Total       int64 `json:"total"`
	Completed   int64 `json:"completed"`
	Downloading int64 `json:"downloading"`
	Failed      int64 `json:"failed"`
	Offline     int64 `json:"offline"`
}

type distributionRequester struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type distributionJob struct {
	ID          string                `json:"id"`
	File        distributionFile      `json:"file"`
	Target      distributionTarget    `json:"target"`
	Status      string                `json:"status"`
	Summary     distributionSummary   `json:"summary"`
	RequestedBy distributionRequester `json:"requested_by"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	CompletedAt *time.Time            `json:"completed_at"`
}

type distributionAgent struct {
	AgentID         string     `json:"agent_id"`
	Hostname        string     `json:"hostname"`
	IPAddress       *string    `json:"ip_address"`
	Status          string     `json:"status"`
	Progress        int        `json:"progress"`
	DownloadedBytes int64      `json:"downloaded_bytes"`
	TotalBytes      int64      `json:"total_bytes"`
	ErrorCode       *string    `json:"error_code"`
	ErrorMessage    *string    `json:"error_message"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

const distributionJobSelect = `SELECT j.id,f.id,f.filename,f.file_size,j.target_type,
	j.room_id,COALESCE(r.name,j.total_targets::text || ' agents'),j.status,
	COUNT(t.id),COUNT(t.id) FILTER (WHERE t.status='COMPLETED'),
	COUNT(t.id) FILTER (WHERE t.status IN ('PENDING','SENT','DOWNLOADING','VERIFYING')),
	COUNT(t.id) FILTER (WHERE t.status='FAILED'),COUNT(t.id) FILTER (WHERE t.status='OFFLINE'),
	u.id,u.username,j.created_at,j.updated_at,j.completed_at
	FROM file_distribution_jobs j
	JOIN files f ON f.id=j.file_id
	JOIN users u ON u.id=j.requested_by
	LEFT JOIN rooms r ON r.id=j.room_id
	LEFT JOIN file_distribution_targets t ON t.job_id=j.id`

const distributionJobGroup = ` GROUP BY j.id,f.id,r.id,u.id`

func scanDistributionJob(scanner interface{ Scan(...interface{}) error }) (distributionJob, error) {
	var job distributionJob
	err := scanner.Scan(&job.ID, &job.File.ID, &job.File.Filename, &job.File.Size,
		&job.Target.Type, &job.Target.RoomID, &job.Target.Label, &job.Status,
		&job.Summary.Total, &job.Summary.Completed, &job.Summary.Downloading,
		&job.Summary.Failed, &job.Summary.Offline, &job.RequestedBy.ID,
		&job.RequestedBy.Username, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	return job, err
}

func distributionFilters(q, status string) (string, []interface{}) {
	clauses := make([]string, 0, 2)
	args := make([]interface{}, 0, 2)
	if q != "" {
		args = append(args, "%"+q+"%")
		clauses = append(clauses, fmt.Sprintf("(f.filename ILIKE $%d OR j.id::text ILIKE $%d OR r.name ILIKE $%d)", len(args), len(args), len(args)))
	}
	if status != "" {
		args = append(args, status)
		clauses = append(clauses, fmt.Sprintf("j.status=$%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// ListFileDistributions returns a newest-first snapshot. The dashboard summary
// intentionally covers every job, independently of the active search/filter.
func ListFileDistributions(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, err := positiveQueryInt(c, "page", 1, 0)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "page must be a positive integer"})
		}
		limit, err := positiveQueryInt(c, "limit", 20, 100)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "limit must be an integer from 1 to 100"})
		}
		q := strings.TrimSpace(c.Query("q"))
		status := strings.ToUpper(strings.TrimSpace(c.Query("status")))
		if status != "" && !distributionListStatuses[status] {
			return c.Status(400).JSON(fiber.Map{"error": "invalid distribution status"})
		}
		if int64(page-1) > (int64(^uint64(0)>>1) / int64(limit)) {
			return c.Status(400).JSON(fiber.Map{"error": "page is too large"})
		}

		where, args := distributionFilters(q, status)
		var total int64
		countQuery := `SELECT COUNT(*) FROM file_distribution_jobs j JOIN files f ON f.id=j.file_id LEFT JOIN rooms r ON r.id=j.room_id` + where
		if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution jobs"})
		}

		listArgs := append([]interface{}{}, args...)
		listArgs = append(listArgs, limit, int64(page-1)*int64(limit))
		query := distributionJobSelect + where + distributionJobGroup +
			fmt.Sprintf(" ORDER BY j.created_at DESC,j.id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
		rows, err := db.Query(query, listArgs...)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution jobs"})
		}
		defer rows.Close()
		jobs := make([]distributionJob, 0)
		for rows.Next() {
			job, err := scanDistributionJob(rows)
			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution jobs"})
			}
			jobs = append(jobs, job)
		}
		if rows.Err() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution jobs"})
		}

		var totalJobs, inProgress, completed, failed int64
		err = db.QueryRow(`SELECT COUNT(*),
			COUNT(*) FILTER (WHERE status='IN_PROGRESS'),
			COUNT(*) FILTER (WHERE status='COMPLETED'),
			COUNT(*) FILTER (WHERE status IN ('PARTIAL_FAILED','FAILED'))
			FROM file_distribution_jobs`).Scan(&totalJobs, &inProgress, &completed, &failed)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution summary"})
		}
		totalPages := int64(0)
		if total > 0 {
			totalPages = (total + int64(limit) - 1) / int64(limit)
		}
		return c.JSON(fiber.Map{
			"jobs":       jobs,
			"summary":    fiber.Map{"total_jobs": totalJobs, "in_progress": inProgress, "completed": completed, "failed": failed},
			"pagination": fiber.Map{"page": page, "limit": limit, "total": total, "total_pages": totalPages},
		})
	}
}

func GetFileDistribution(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		jobID := c.Params("jobId")
		if _, err := uuid.Parse(jobID); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "distribution job not found"})
		}
		job, err := scanDistributionJob(db.QueryRow(distributionJobSelect+` WHERE j.id=$1`+distributionJobGroup, jobID))
		if errors.Is(err, sql.ErrNoRows) {
			return c.Status(404).JSON(fiber.Map{"error": "distribution job not found"})
		}
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution job"})
		}

		rows, err := db.Query(`SELECT t.agent_id,a.hostname,a.ip_address::text,t.status,t.progress,
			t.downloaded_bytes,f.file_size,t.error_code,t.error_message,t.started_at,t.completed_at,t.updated_at
			FROM file_distribution_targets t
			JOIN agents a ON a.id=t.agent_id
			JOIN file_distribution_jobs j ON j.id=t.job_id
			JOIN files f ON f.id=j.file_id
			WHERE t.job_id=$1 ORDER BY a.hostname,a.id`, jobID)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution targets"})
		}
		defer rows.Close()
		agents := make([]distributionAgent, 0)
		for rows.Next() {
			var agent distributionAgent
			if err := rows.Scan(&agent.AgentID, &agent.Hostname, &agent.IPAddress, &agent.Status,
				&agent.Progress, &agent.DownloadedBytes, &agent.TotalBytes, &agent.ErrorCode,
				&agent.ErrorMessage, &agent.StartedAt, &agent.CompletedAt, &agent.UpdatedAt); err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution targets"})
			}
			agents = append(agents, agent)
		}
		if rows.Err() != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read distribution targets"})
		}

		return c.JSON(fiber.Map{
			"id": job.ID, "file": job.File, "target": job.Target, "status": job.Status,
			"summary": job.Summary, "agents": agents, "requested_by": job.RequestedBy,
			"created_at": job.CreatedAt, "updated_at": job.UpdatedAt, "completed_at": job.CompletedAt,
		})
	}
}
