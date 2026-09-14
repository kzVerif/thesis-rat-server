package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func TestEnrollmentRejectsInvalidInput(t *testing.T) {
	app := fiber.New()
	app.Post("/tokens", CreateToken(nil))
	app.Patch("/tokens/:id", UpdateToken(nil))
	app.Get("/tokens/:id", GetToken(nil))
	app.Post("/validate", ValidateToken(nil))
	app.Post("/register", RegisterAgent(nil))
	id := uuid.NewString()
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/tokens", `{"created_by":"forged"}`},
		{"POST", "/tokens", `{"agent_id":"forged"}`},
		{"POST", "/tokens", `{"max_use":0}`},
		{"POST", "/tokens", `{"max_use":"2"}`},
		{"POST", "/tokens", `{"max_use":1.5}`},
		{"POST", "/tokens", `{"expires_at":"2020-01-01T00:00:00Z"}`},
		{"POST", "/tokens", `null`},
		{"POST", "/tokens", `{} {}`},
		{"PATCH", "/tokens/" + id, `{"is_revoked":false}`},
		{"PATCH", "/tokens/" + id, `{}`},
		{"GET", "/tokens/invalid", ``},
		{"POST", "/validate", `{}`},
		{"POST", "/register", `{"hostname":"pc","mac_address":"aa:bb:cc:dd:ee:ff"}`},
		{"POST", "/register", `{"token":"secret","hostname":"pc"}`},
		{"POST", "/register", `{"token":"secret","hostname":"pc","mac_address":"invalid"}`},
		{"POST", "/register", `{"token":"secret","hostname":"pc","mac_address":"aa:bb:cc:dd:ee:ff","status":"ONLINE"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Errorf("%s %s %s: got %d", tc.method, tc.path, tc.body, res.StatusCode)
		}
	}
}

// TEST_DATABASE_URL must point at a disposable PostgreSQL database and use
// lib/pq keyword DSN syntax. Each test creates/drops only its own random schema.
func enrollmentTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(`CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "enrollment_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	db, err := sql.Open("postgres", dsn+" search_path="+schema+",public")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		_, err := admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return db
}

func TestEnrollmentWorkflowPostgres(t *testing.T) {
	db := enrollmentTestDatabase(t)
	schema, err := os.ReadFile("../schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	var role, user string
	if err := db.QueryRow(`INSERT INTO roles(name) VALUES('ENROLLMENT_MANAGER') RETURNING id`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO role_permissions SELECT $1,id FROM permissions WHERE code='tokens.manage'`, role); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO users(username,password_hash,role_id) VALUES('creator','unused',$1) RETURNING id`, role).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_sessions(user_id,token_hash,expires_at) VALUES($1,$2,NOW()+INTERVAL '1 day')`, user, hashToken("test-session")); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Use("/api", AuditRequests(db))
	app.Post("/api/agents/register", RegisterAgent(db))
	app.Post("/api/tokens/validate", ValidateToken(db))
	api := app.Group("/api", RequireAuth(db))
	tokens := api.Group("/tokens", RequirePermission(db, TokensManagePermission))
	tokens.Get("/", ListTokens(db))
	tokens.Get("/:id", GetToken(db))
	tokens.Post("/", CreateToken(db))
	tokens.Patch("/:id", UpdateToken(db))
	tokens.Put("/:id", UpdateToken(db))
	tokens.Delete("/:id", RevokeToken(db))
	request := func(method, path, body string, authenticated bool) (int, map[string]interface{}) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.AddCookie(&http.Cookie{Name: hostCookieName, Value: "test-session"})
		}
		res, err := app.Test(req, 10000)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, out
	}
	if status, _ := request("GET", "/api/tokens", "", false); status != 401 {
		t.Fatalf("unauthenticated status=%d", status)
	}
	status, created := request("POST", "/api/tokens", `{"max_use":3}`, true)
	if status != 201 || created["created_by"] != user {
		t.Fatalf("create: %d %+v", status, created)
	}
	id, secret := created["id"].(string), created["token"].(string)
	var stored string
	if err := db.QueryRow(`SELECT token_hash FROM tokens WHERE id=$1`, id).Scan(&stored); err != nil || stored != hashToken(secret) || stored == secret {
		t.Fatalf("hash storage: %v", err)
	}
	for i := 0; i < 2; i++ {
		if status, _ := request("POST", "/api/tokens/validate", fmt.Sprintf(`{"token":%q}`, secret), false); status != 200 {
			t.Fatalf("preflight status=%d", status)
		}
	}
	var used int
	if err := db.QueryRow(`SELECT used_count FROM tokens WHERE id=$1`, id).Scan(&used); err != nil || used != 0 {
		t.Fatalf("preflight consumed a use: %d %v", used, err)
	}
	// Twelve concurrent agents compete for three slots. No human session needed.
	var wg sync.WaitGroup
	statuses := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"token":%q,"hostname":"pc-%d","mac_address":"aa:bb:cc:dd:ee:%02x"}`, secret, i, i)
			req := httptest.NewRequest("POST", "/api/agents/register", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req, 10000)
			if err != nil {
				t.Error(err)
				statuses <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			statuses <- res.StatusCode
		}(i)
	}
	wg.Wait()
	close(statuses)
	successes := 0
	for status := range statuses {
		if status == 201 {
			successes++
		} else if status != 403 {
			t.Errorf("unexpected registration status %d", status)
		}
	}
	if successes != 3 {
		t.Fatalf("successful enrollments=%d", successes)
	}
	if err := db.QueryRow(`SELECT used_count FROM tokens WHERE id=$1`, id).Scan(&used); err != nil || used != 3 {
		t.Fatalf("usage=%d err=%v", used, err)
	}
	if status, _ := request("PATCH", "/api/tokens/"+id, `{"max_use":2}`, true); status != 409 {
		t.Fatalf("lowering quota status=%d", status)
	}
	if status, _ := request("PUT", "/api/tokens/"+id, `{"max_use":5,"expires_at":null}`, true); status != 200 {
		t.Fatalf("update status=%d", status)
	}
	if status, _ := request("PATCH", "/api/tokens/"+id, `{"max_use":null}`, true); status != 200 {
		t.Fatalf("clear quota status=%d", status)
	}
	// Invalid room rolls back both the new agent and the consumed quota.
	status, _ = request("POST", "/api/agents/register", fmt.Sprintf(`{"token":%q,"hostname":"bad-room","mac_address":"ff:ee:dd:cc:bb:aa","room_id":%q}`, secret, uuid.NewString()), false)
	if status != 400 {
		t.Fatalf("bad room status=%d", status)
	}
	var mac string
	if err := db.QueryRow(`SELECT mac_address FROM agents LIMIT 1`).Scan(&mac); err != nil {
		t.Fatal(err)
	}
	status, _ = request("POST", "/api/agents/register", fmt.Sprintf(`{"token":%q,"hostname":"duplicate","mac_address":%q}`, secret, strings.ToUpper(mac)), false)
	if status != 409 {
		t.Fatalf("duplicate status=%d", status)
	}
	if err := db.QueryRow(`SELECT used_count FROM tokens WHERE id=$1`, id).Scan(&used); err != nil || used != 3 {
		t.Fatalf("failed enrollment consumed quota: %d %v", used, err)
	}
	status, listing := request("GET", "/api/tokens?page=1&limit=1", "", true)
	encoded, _ := json.Marshal(listing)
	if status != 200 || strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "token_hash") {
		t.Fatalf("listing leaked credential: %d", status)
	}
	status, item := request("GET", "/api/tokens/"+id, "", true)
	if status != 200 || item["used_count"] != float64(3) || item["max_use"] != nil {
		t.Fatalf("get: %d %+v", status, item)
	}
	if status, _ := request("DELETE", "/api/tokens/"+id, "", true); status != 200 {
		t.Fatalf("revoke status=%d", status)
	}
	if status, _ := request("POST", "/api/tokens/validate", fmt.Sprintf(`{"token":%q}`, secret), false); status != 403 {
		t.Fatalf("revoked validation status=%d", status)
	}
	var revoked bool
	if err := db.QueryRow(`SELECT is_revoked FROM tokens WHERE id=$1`, id).Scan(&revoked); err != nil || !revoked {
		t.Fatalf("revoked row missing: %v", err)
	}
	var leaks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM logs WHERE detail::text LIKE $1 OR (action='POST /api/agents/register' AND detail::text LIKE $2)`, "%"+secret+"%", "%"+id+"%").Scan(&leaks); err != nil || leaks != 0 {
		t.Fatalf("credential/association leaked to logs: %d %v", leaks, err)
	}
	// Having users.manage alone must no longer authorize token management.
	if _, err := db.Exec(`DELETE FROM role_permissions WHERE role_id=$1`, role); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO role_permissions SELECT $1,id FROM permissions WHERE code='users.manage'`, role); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST", "PATCH", "PUT", "DELETE"} {
		path := "/api/tokens"
		if method == "PATCH" || method == "PUT" || method == "DELETE" {
			path += "/" + id
		}
		if status, _ := request(method, path, `{}`, true); status != 403 {
			t.Errorf("%s permission status=%d", method, status)
		}
	}
}

func TestEnrollmentMigrationPostgres(t *testing.T) {
	db := enrollmentTestDatabase(t)
	_, err := db.Exec(`CREATE TABLE users(id UUID PRIMARY KEY); CREATE TABLE agents(id UUID PRIMARY KEY,mac_address VARCHAR(17));
		CREATE TABLE roles(id UUID PRIMARY KEY,name TEXT); CREATE TABLE permissions(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),code TEXT UNIQUE,description TEXT);
		CREATE TABLE role_permissions(role_id UUID,permission_id UUID,PRIMARY KEY(role_id,permission_id));
		CREATE TABLE tokens(id UUID PRIMARY KEY DEFAULT gen_random_uuid(),user_id UUID REFERENCES users(id),agent_id UUID REFERENCES agents(id),
		token TEXT NOT NULL UNIQUE,token_type TEXT NOT NULL,max_use INTEGER,used_count INTEGER NOT NULL DEFAULT 0,expires_at TIMESTAMPTZ,
		is_revoked BOOLEAN NOT NULL DEFAULT FALSE,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		INSERT INTO tokens(token,token_type,max_use,used_count) VALUES('legacy-secret','api_key',10,2)`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../migrations/20260913_agent_enrollment_tokens.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var hash string
	var creator sql.NullString
	var revoked bool
	var used int
	if err := db.QueryRow(`SELECT token_hash,created_by,is_revoked,used_count FROM tokens`).Scan(&hash, &creator, &revoked, &used); err != nil {
		t.Fatal(err)
	}
	if hash != hashToken("legacy-secret") || creator.Valid || !revoked || used != 2 {
		t.Fatal("legacy history incorrectly migrated")
	}
	var oldColumns int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='tokens' AND column_name IN ('token','user_id','agent_id','token_type')`).Scan(&oldColumns); err != nil || oldColumns != 0 {
		t.Fatalf("old columns=%d %v", oldColumns, err)
	}
}
