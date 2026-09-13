package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

func ParseLogRetentionDays(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 90, nil
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days < 1 || days > 3650 {
		return 0, fmt.Errorf("LOG_RETENTION_DAYS must be between 1 and 3650")
	}
	return days, nil
}

const deleteExpiredLogsSQL = `DELETE FROM logs WHERE id IN (
	SELECT id FROM logs WHERE created_at < $1
	ORDER BY created_at,id LIMIT $2 FOR UPDATE SKIP LOCKED
)`

// RunLogRetention runs at startup and every 24 hours while the server is alive.
// Each batch commits independently; a failed pass is retried the following day.
func RunLogRetention(ctx context.Context, db *sql.DB, days int) {
	if days < 1 || days > 3650 {
		log.Print("invalid log retention days")
		return
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		passCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		deleted, err := purgeExpiredLogs(passCtx, db, time.Now().UTC().AddDate(0, 0, -days))
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("log retention failed after deleting %d rows: %v", deleted, err)
		}
		if deleted > 0 {
			log.Printf("log retention deleted %d expired rows", deleted)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func purgeExpiredLogs(ctx context.Context, db *sql.DB, cutoff time.Time) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, err := db.ExecContext(batchCtx, deleteExpiredLogsSQL, cutoff, 1000)
		cancel()
		if err != nil {
			return total, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
		if n < 1000 {
			return total, nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return total, ctx.Err()
		case <-timer.C:
		}
	}
}
