package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// A small database/sql driver exercises row decoding, transactions and bound
// arguments without requiring a running PostgreSQL instance.
type logsTestDB struct {
	query func(string, []driver.NamedValue) (driver.Rows, error)
	exec  func(string, []driver.NamedValue) (driver.Result, error)
}

func (d *logsTestDB) Connect(context.Context) (driver.Conn, error) { return d, nil }
func (d *logsTestDB) Driver() driver.Driver                        { return d }
func (d *logsTestDB) Open(string) (driver.Conn, error)             { return d, nil }
func (d *logsTestDB) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (d *logsTestDB) Close() error                                                 { return nil }
func (d *logsTestDB) Begin() (driver.Tx, error)                                    { return d, nil }
func (d *logsTestDB) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) { return d, nil }
func (d *logsTestDB) Commit() error                                                { return nil }
func (d *logsTestDB) Rollback() error                                              { return nil }
func (d *logsTestDB) QueryContext(_ context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	return d.query(q, a)
}
func (d *logsTestDB) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	return d.exec(q, a)
}

type logsTestRows struct {
	columns []string
	values  [][]driver.Value
}

func (r *logsTestRows) Columns() []string { return r.columns }
func (r *logsTestRows) Close() error      { return nil }
func (r *logsTestRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func TestLogsListDecodesNullableFieldsAndBindsFilters(t *testing.T) {
	const id = "bca376bb-7e48-4d75-afaa-e6b75b597c60"
	var queries int
	db := sql.OpenDB(&logsTestDB{query: func(q string, args []driver.NamedValue) (driver.Rows, error) {
		queries++
		if !strings.Contains(q, "l.action=$1") || args[0].Value != "DELETE /api/agents/:id" {
			t.Errorf("filter not bound: %s %+v", q, args)
		}
		if queries == 1 {
			return &logsTestRows{[]string{"count"}, [][]driver.Value{{int64(1)}}}, nil
		}
		if len(args) != 3 || args[1].Value != int64(20) || args[2].Value != int64(0) {
			t.Errorf("unexpected pagination: %+v", args)
		}
		return &logsTestRows{[]string{"id", "user_id", "username", "display_name", "action", "target_agent_id", "hostname", "detail", "ip", "created"}, [][]driver.Value{
			{id, nil, nil, nil, "DELETE /api/agents/:id", nil, nil, []byte(`{"target_agent_id":"` + id + `"}`), nil, time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)},
		}}, nil
	}})
	defer db.Close()
	app := fiber.New()
	app.Get("/logs", ListLogs(db))
	resp, err := app.Test(httptest.NewRequest("GET", "/logs?action=DELETE%20%2Fapi%2Fagents%2F%3Aid", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Logs       []map[string]interface{} `json:"logs"`
		Pagination struct {
			Total int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || len(body.Logs) != 1 || body.Pagination.Total != 1 {
		t.Fatalf("unexpected response: %d %+v", resp.StatusCode, body)
	}
	if body.Logs[0]["user_id"] != nil || body.Logs[0]["detail"].(map[string]interface{})["target_agent_id"] != id {
		t.Fatalf("incorrect nullable fields or detail: %+v", body.Logs[0])
	}
}

func TestAuditWritesDatabaseIdentifiersAndJSON(t *testing.T) {
	const id = "bca376bb-7e48-4d75-afaa-e6b75b597c60"
	var writes int
	db := sql.OpenDB(&logsTestDB{exec: func(q string, args []driver.NamedValue) (driver.Result, error) {
		writes++
		if !strings.Contains(q, "INSERT INTO logs") || len(args) != 5 || args[0].Value != id || args[2].Value != id {
			t.Errorf("unexpected insert: %s %+v", q, args)
		}
		var detail map[string]interface{}
		if err := json.Unmarshal([]byte(args[3].Value.(string)), &detail); err != nil {
			t.Error(err)
		}
		if detail["status_code"] != float64(204) || detail["actor_id"] != id {
			t.Errorf("unexpected detail: %+v", detail)
		}
		return driver.RowsAffected(1), nil
	}})
	defer db.Close()
	app := fiber.New()
	app.Use("/api", AuditRequests(db))
	app.Delete("/api/agents/:id", func(c *fiber.Ctx) error { c.Locals(authUserLocal, &authUser{ID: id}); return c.SendStatus(204) })
	resp, err := app.Test(httptest.NewRequest("DELETE", "/api/agents/"+id, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if writes != 1 || resp.StatusCode != 204 {
		t.Fatalf("writes=%d status=%d", writes, resp.StatusCode)
	}
}

func TestLogsPermissionDeniedAndMissingRecord(t *testing.T) {
	db := sql.OpenDB(&logsTestDB{query: func(q string, _ []driver.NamedValue) (driver.Rows, error) {
		if strings.Contains(q, "SELECT EXISTS") {
			return &logsTestRows{[]string{"exists"}, [][]driver.Value{{false}}}, nil
		}
		return &logsTestRows{columns: []string{"id"}}, nil
	}})
	defer db.Close()
	app := fiber.New()
	app.Get("/denied", func(c *fiber.Ctx) error {
		c.Locals(authUserLocal, &authUser{RoleID: AdministratorRoleID})
		return c.Next()
	}, RequirePermission(db, LogsReadPermission), ListLogs(db))
	app.Get("/logs/:id", GetLog(db))
	for path, want := range map[string]int{"/denied": 403, "/logs/bca376bb-7e48-4d75-afaa-e6b75b597c60": 404} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: status=%d want=%d", path, resp.StatusCode, want)
		}
	}
}

func TestAuditRequestsRecordsOutcomesWithoutSecrets(t *testing.T) {
	const id = "bca376bb-7e48-4d75-afaa-e6b75b597c60"
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"POST", "/api/agents", 201}, {"DELETE", "/api/agents/" + id, 204},
		{"POST", "/api/auth/login", 401},
		{"GET", "/api/error", 503},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			var events []auditEvent
			app := fiber.New()
			app.Use("/api", auditRequests(func(e auditEvent) error { events = append(events, e); return nil }))
			app.Post("/api/agents", func(c *fiber.Ctx) error {
				c.Locals(authUserLocal, &authUser{ID: id, Username: "operator", PasswordHash: "secret-hash"})
				return c.Status(201).JSON(fiber.Map{"id": id, "password": "response-secret"})
			})
			app.Delete("/api/agents/:id", func(c *fiber.Ctx) error { return c.SendStatus(204) })
			app.Get("/api/agents/:id", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"id": id}) })
			app.Post("/api/auth/login", func(c *fiber.Ctx) error { return c.SendStatus(401) })
			app.Get("/api/error", func(c *fiber.Ctx) error { return fiber.NewError(503, "unavailable") })
			req := httptest.NewRequest(tc.method, tc.path+"?token=query-secret", strings.NewReader(`{"password":"body-secret"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", "__Host-session=cookie-secret")
			req.Header.Set("Authorization", "Bearer header-secret")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status || len(events) != 1 {
				t.Fatalf("status=%d events=%d", resp.StatusCode, len(events))
			}
			event := events[0]
			if event.Detail["status_code"] != tc.status || event.Detail["success"] != (tc.status < 400) {
				t.Fatalf("wrong outcome: %+v", event)
			}
			if strings.Contains(tc.path, "agents") && event.AgentID != id {
				t.Fatalf("missing target: %+v", event)
			}
			if tc.status == 201 && event.UserID != id {
				t.Fatal("missing actor")
			}
			encoded, _ := json.Marshal(event)
			if strings.Contains(string(encoded), "secret") {
				t.Fatalf("secret leaked: %s", encoded)
			}
		})
	}
}

func TestAuditWriteFailurePreservesCommittedResponse(t *testing.T) {
	app := fiber.New()
	app.Use(auditRequests(func(auditEvent) error { return errors.New("database unavailable") }))
	app.Post("/api/rooms", func(c *fiber.Ctx) error { return c.Status(201).SendString("created") })
	resp, err := app.Test(httptest.NewRequest("POST", "/api/rooms", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 201 || string(body) != "created" {
		t.Fatalf("response changed: %d %s", resp.StatusCode, body)
	}
}

func TestLogsRequireAuthenticationAndPermission(t *testing.T) {
	app := fiber.New()
	var recorded auditEvent
	app.Use("/api", auditRequests(func(e auditEvent) error { recorded = e; return nil }))
	api := app.Group("/api", RequireAuth(nil))
	api.Get("/logs", RequirePermission(nil, LogsReadPermission), ListLogs(nil))
	resp, err := app.Test(httptest.NewRequest("GET", "/api/logs", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 || recorded.Detail["status_code"] != 401 {
		t.Fatalf("expected logged 401, got %d %+v", resp.StatusCode, recorded)
	}
	// The permission middleware must also reject missing auth locals directly.
	other := fiber.New()
	other.Get("/logs", RequirePermission(nil, LogsReadPermission), ListLogs(nil))
	resp, err = other.Test(httptest.NewRequest("GET", "/logs", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestLogsRejectInvalidFiltersBeforeDatabaseAccess(t *testing.T) {
	app := fiber.New()
	app.Get("/logs", ListLogs(nil))
	app.Get("/logs/:id", GetLog(nil))
	for _, path := range []string{
		"/logs?page=0", "/logs?page=abc", "/logs?limit=101", "/logs?limit=-1",
		"/logs?page=9223372036854775807&limit=100", "/logs?user_id=invalid", "/logs?target_agent_id=invalid",
		"/logs?from=yesterday", "/logs?to=2026-09-13", "/logs?from=2026-09-14T00:00:00Z&to=2026-09-13T00:00:00Z",
		"/logs?action=" + strings.Repeat("x", 101), "/logs/invalid",
	} {
		t.Run(path, func(t *testing.T) {
			resp, err := app.Test(httptest.NewRequest("GET", path, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Fatalf("status=%d", resp.StatusCode)
			}
		})
	}
}
