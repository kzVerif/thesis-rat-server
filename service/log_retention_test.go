package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestAuditPolicy(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"} {
		for _, status := range []int{200, 201, 204, 400, 401, 403, 404, 409, 429, 500, 503} {
			want := method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" || status == 401 || status == 403 || status == 429 || status >= 500
			if got := shouldAudit(method, status); got != want {
				t.Errorf("%s %d = %v, want %v", method, status, got, want)
			}
		}
	}
}

func TestRoutineReadsAndRepeatedFailuresDoNotWrite(t *testing.T) {
	writes := 0
	app := fiber.New()
	app.Use(auditRequests(func(auditEvent) error { writes++; return nil }))
	app.Get("/api/logs", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"logs": []string{}}) })
	app.Get("/api/agents", func(c *fiber.Ctx) error { return c.SendStatus(403) })
	app.Post("/api/rooms", func(c *fiber.Ctx) error { return c.SendStatus(201) })
	for _, req := range []struct {
		method, path string
		count        int
	}{{"GET", "/api/logs", 0}, {"GET", "/api/logs", 0}, {"GET", "/api/agents", 1}, {"GET", "/api/agents", 1}, {"POST", "/api/rooms", 2}, {"POST", "/api/rooms", 3}} {
		resp, err := app.Test(httptest.NewRequest(req.method, req.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if writes != req.count {
			t.Fatalf("%s %s writes=%d want=%d", req.method, req.path, writes, req.count)
		}
	}
}

func TestFailureGateWindowAndCapacity(t *testing.T) {
	g := &auditFailureGate{}
	now := time.Now()
	key := auditFailureKey{ip: "127.0.0.1", action: "POST /api/auth/login", status: 401}
	if !g.allow(key, now) || g.allow(key, now.Add(time.Second)) || !g.allow(key, now.Add(time.Minute)) {
		t.Fatal("incorrect window behavior")
	}
	for i := 1; i < 4096; i++ {
		other := key
		other.user = string(rune(i))
		if !g.allow(other, now.Add(time.Minute)) {
			t.Fatal("early capacity limit")
		}
	}
	other := key
	other.user = "overflow"
	if g.allow(other, now.Add(time.Minute)) || len(g.seen) != 4096 {
		t.Fatal("capacity not enforced")
	}
	if !g.allow(other, now.Add(2*time.Minute)) {
		t.Fatal("capacity not reset")
	}
}

func TestRetentionConfig(t *testing.T) {
	for raw, want := range map[string]int{"": 90, "90": 90, " 30 ": 30, "3650": 3650} {
		got, err := ParseLogRetentionDays(raw)
		if err != nil || got != want {
			t.Errorf("%q: %d %v", raw, got, err)
		}
	}
	for _, raw := range []string{"0", "-1", "3651", "abc", "1.5"} {
		if _, err := ParseLogRetentionDays(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestRetentionBatchesAndStopsOnError(t *testing.T) {
	for _, fail := range []bool{false, true} {
		cutoff := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		calls := 0
		db := sql.OpenDB(&logsTestDB{exec: func(q string, args []driver.NamedValue) (driver.Result, error) {
			calls++
			if q != deleteExpiredLogsSQL || !args[0].Value.(time.Time).Equal(cutoff) || args[1].Value != int64(1000) {
				t.Errorf("unexpected deletion: %s %+v", q, args)
			}
			if calls == 1 {
				return driver.RowsAffected(1000), nil
			}
			if fail {
				return nil, errors.New("database failure")
			}
			return driver.RowsAffected(3), nil
		}})
		n, err := purgeExpiredLogs(context.Background(), db, cutoff)
		db.Close()
		want := int64(1003)
		if fail {
			want = 1000
		}
		if n != want || (err != nil) != fail || calls != 2 {
			t.Fatalf("deleted=%d err=%v calls=%d", n, err, calls)
		}
	}
}

func TestRetentionHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := purgeExpiredLogs(ctx, nil, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}
