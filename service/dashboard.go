package service

import (
	"database/sql"
	"time"

	"github.com/gofiber/fiber/v2"
)

// GetDashboard returns one consistent, read-only snapshot for the main dashboard.
// It intentionally returns aggregates rather than rows from sensitive tables.
func GetDashboard(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var out struct {
			GeneratedAt                                                                                                  time.Time
			AgentsTotal, AgentsOnline, AgentsOffline, AgentsWarning, AgentsDisabled                                      int64
			RoomsTotal, UsersTotal, UsersActive, UsersDisabled, UsersLocked                                              int64
			TokensTotal, TokensActive, TokensRevoked, TokensExpired, TokensExhausted, TokenUses                          int64
			FilesTotal, FilesBytes                                                                                       int64
			ScansTotal, ScansCompleted, ScansFailed, ThreatsFound                                                        int64
			DistributionsTotal, DistributionsPending, DistributionsProgress, DistributionsCompleted, DistributionsFailed int64
			Activity24h                                                                                                  int64
		}
		err := db.QueryRowContext(c.UserContext(), `SELECT
			NOW(),
			(SELECT COUNT(*) FROM agents),
			(SELECT COUNT(*) FROM agents WHERE status='ONLINE'),
			(SELECT COUNT(*) FROM agents WHERE status='OFFLINE'),
			(SELECT COUNT(*) FROM agents WHERE status='WARNING'),
			(SELECT COUNT(*) FROM agents WHERE status='DISABLED'),
			(SELECT COUNT(*) FROM rooms),
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE status='ACTIVE'),
			(SELECT COUNT(*) FROM users WHERE status='DISABLED'),
			(SELECT COUNT(*) FROM users WHERE status='LOCKED'),
			(SELECT COUNT(*) FROM tokens),
			(SELECT COUNT(*) FROM tokens WHERE NOT is_revoked AND (expires_at IS NULL OR expires_at>NOW()) AND (max_use IS NULL OR used_count<max_use)),
			(SELECT COUNT(*) FROM tokens WHERE is_revoked),
			(SELECT COUNT(*) FROM tokens WHERE NOT is_revoked AND expires_at IS NOT NULL AND expires_at<=NOW()),
			(SELECT COUNT(*) FROM tokens WHERE NOT is_revoked AND max_use IS NOT NULL AND used_count>=max_use),
			(SELECT COALESCE(SUM(used_count),0) FROM tokens),
			(SELECT COUNT(*) FROM files),
			(SELECT COALESCE(SUM(file_size),0) FROM files),
			(SELECT COUNT(*) FROM av_scan_results),
			(SELECT COUNT(*) FROM av_scan_results WHERE status='COMPLETED'),
			(SELECT COUNT(*) FROM av_scan_results WHERE status='FAILED'),
			(SELECT COALESCE(SUM(threats_found),0) FROM av_scan_results),
			(SELECT COUNT(*) FROM file_distribution_jobs),
			(SELECT COUNT(*) FROM file_distribution_jobs WHERE status IN ('PENDING','DISPATCHING')),
			(SELECT COUNT(*) FROM file_distribution_jobs WHERE status IN ('IN_PROGRESS','PARTIAL_FAILED')),
			(SELECT COUNT(*) FROM file_distribution_jobs WHERE status='COMPLETED'),
			(SELECT COUNT(*) FROM file_distribution_jobs WHERE status IN ('FAILED','CANCELLED')),
			(SELECT COUNT(*) FROM logs WHERE created_at>=NOW()-INTERVAL '24 hours')`).Scan(
			&out.GeneratedAt,
			&out.AgentsTotal, &out.AgentsOnline, &out.AgentsOffline, &out.AgentsWarning, &out.AgentsDisabled,
			&out.RoomsTotal, &out.UsersTotal, &out.UsersActive, &out.UsersDisabled, &out.UsersLocked,
			&out.TokensTotal, &out.TokensActive, &out.TokensRevoked, &out.TokensExpired, &out.TokensExhausted, &out.TokenUses,
			&out.FilesTotal, &out.FilesBytes,
			&out.ScansTotal, &out.ScansCompleted, &out.ScansFailed, &out.ThreatsFound,
			&out.DistributionsTotal, &out.DistributionsPending, &out.DistributionsProgress,
			&out.DistributionsCompleted, &out.DistributionsFailed, &out.Activity24h)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "unable to read dashboard data"})
		}
		return c.JSON(fiber.Map{
			"generated_at":       out.GeneratedAt,
			"agents":             fiber.Map{"total": out.AgentsTotal, "online": out.AgentsOnline, "offline": out.AgentsOffline, "warning": out.AgentsWarning, "disabled": out.AgentsDisabled},
			"rooms":              fiber.Map{"total": out.RoomsTotal},
			"users":              fiber.Map{"total": out.UsersTotal, "active": out.UsersActive, "disabled": out.UsersDisabled, "locked": out.UsersLocked},
			"tokens":             fiber.Map{"total": out.TokensTotal, "active": out.TokensActive, "revoked": out.TokensRevoked, "expired": out.TokensExpired, "exhausted": out.TokensExhausted, "uses": out.TokenUses},
			"files":              fiber.Map{"total": out.FilesTotal, "total_bytes": out.FilesBytes},
			"antivirus":          fiber.Map{"total_scans": out.ScansTotal, "completed": out.ScansCompleted, "failed": out.ScansFailed, "threats_found": out.ThreatsFound},
			"file_distributions": fiber.Map{"total": out.DistributionsTotal, "pending": out.DistributionsPending, "in_progress": out.DistributionsProgress, "completed": out.DistributionsCompleted, "failed": out.DistributionsFailed},
			"activity":           fiber.Map{"last_24_hours": out.Activity24h},
		})
	}
}
