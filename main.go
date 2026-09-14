package main

import (
	"context"
	"log"
	"os"
	"rat-server/service"
	"strings"

	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"
)

func main() {
	app := fiber.New()
	app.Use(localCORS)
	db = SetupDatabase()
	defer db.Close()
	retentionDays, err := service.ParseLogRetentionDays(os.Getenv("LOG_RETENTION_DAYS"))
	if err != nil {
		log.Fatal(err)
	}
	retentionCtx, stopRetention := context.WithCancel(context.Background())
	defer stopRetention()
	go service.RunLogRetention(retentionCtx, db, retentionDays)

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	fileStore, err := service.NewFileStore(db, uploadDir)
	if err != nil {
		log.Fatal(err)
	}

	app.Use("/api", service.AuditRequests(db))
	// First enrollment authenticates with a token before session middleware.
	app.Post("/api/agents/register", service.RegisterAgent(db))
	app.Post("/api/tokens/validate", service.ValidateToken(db))
	auth := app.Group("/api/auth")
	auth.Post("/register", service.CreateUser(db))
	auth.Post("/login", service.Login(db))

	protectedAuth := auth.Group("", service.RequireAuth(db))
	protectedAuth.Post("/logout", service.Logout(db))
	protectedAuth.Post("/logout-all", service.LogoutAll(db))
	protectedAuth.Get("/me", service.Me(db))
	protectedAuth.Get("/sessions", service.Sessions(db))
	protectedAuth.Delete("/sessions/:id", service.DeleteSession(db))
	protectedAuth.Post("/change-password", service.ChangePassword(db))

	// ใส่ API ตรงตำแหน่งนี้

	api := app.Group("/api", service.RequireAuth(db))
	dashboard := api.Group("/dashboard")
	dashboard.Get("/", service.GetDashboard(db))
	logs := api.Group("/logs", service.RequirePermission(db, service.LogsReadPermission))
	logs.Get("/", service.ListLogs(db))
	logs.Get("/:id", service.GetLog(db))
	roles := api.Group("/roles")
	roles.Get("/", service.ListRoles(db))
	roles.Get("/:id", service.GetRole(db))
	roles.Post("/", service.RequirePermission(db, service.RoleManagePermission), service.CreateRole(db))
	roles.Put("/:id", service.RequirePermission(db, service.RoleManagePermission), service.UpdateRole(db))
	roles.Delete("/:id", service.RequirePermission(db, service.RoleManagePermission), service.DeleteRole(db))

	permissions := api.Group("/permissions")
	permissions.Get("/", service.ListPermissions(db))
	permissions.Get("/:id", service.GetPermission(db))
	permissions.Post("/", service.RequireAdministrator, service.CreatePermission(db))
	permissions.Put("/:id", service.RequireAdministrator, service.UpdatePermission(db))
	permissions.Delete("/:id", service.RequireAdministrator, service.DeletePermission(db))

	rooms := api.Group("/rooms", service.RequirePermission(db, service.RoomsManagePermission))
	rooms.Get("/", service.ListRooms(db))
	rooms.Get("/:id", service.GetRoom(db))
	rooms.Post("/", service.CreateRoom(db))
	rooms.Put("/:id", service.UpdateRoom(db))
	rooms.Delete("/:id", service.DeleteRoom(db))

	users := api.Group("/users", service.RequirePermission(db, service.UsersManagePermission))
	users.Get("/", service.ListUsers(db))
	users.Get("/:id", service.GetUser(db))
	users.Post("/", service.CreateManagedUser(db))
	users.Put("/:id", service.UpdateUser(db))
	users.Delete("/:id", service.DeleteUser(db))

	// Tokens management
	tokens := api.Group("/tokens", service.RequirePermission(db, service.TokensManagePermission))
	tokens.Get("/", service.ListTokens(db))
	tokens.Get("/:id", service.GetToken(db))
	tokens.Post("/", service.CreateToken(db))
	tokens.Put("/:id", service.UpdateToken(db))
	tokens.Patch("/:id", service.UpdateToken(db))
	tokens.Delete("/:id", service.RevokeToken(db))

	agents := api.Group("/agents")
	agents.Get("/", service.RequireAnyPermission(db, service.AgentsManagePermission, service.AgentsReadPermission), service.ListAgents(db))
	agents.Get("/:id", service.RequireAnyPermission(db, service.AgentsManagePermission, service.AgentsReadPermission), service.GetAgent(db))
	agents.Post("/", service.RequirePermission(db, service.AgentsManagePermission), service.CreateAgent(db))
	agents.Put("/:id", service.RequireAnyPermission(db, service.AgentsManagePermission, service.AgentsEditPermission), service.UpdateAgent(db))
	agents.Delete("/:id", service.RequireAnyPermission(db, service.AgentsManagePermission, service.AgentsDeletePermission), service.DeleteAgent(db))

	files := api.Group("/files")
	scans := api.Group("/av-scan-results", service.RequirePermission(db, service.AVReadPermission))
	scans.Get("/", service.ListAVScanResults(db))
	scans.Get("/:id", service.GetAVScanResult(db))

	files.Get("/", service.RequirePermission(db, service.FilesManagePermission), fileStore.List)
	files.Post("/upload", service.RequireAnyPermission(db, service.FilesManagePermission, service.FilesUploadPermission), fileStore.Upload)
	files.Patch("/:filename", service.RequirePermission(db, service.FilesManagePermission), fileStore.Rename)
	files.Delete("/:filename", service.RequireAnyPermission(db, service.FilesManagePermission, service.FilesDeletePermission), fileStore.Delete)

	distributions := api.Group("/file-distributions", service.RequirePermission(db, service.FilesDistributePermission))
	distributions.Get("/", service.ListFileDistributions(db))
	distributions.Get("/:jobId", service.GetFileDistribution(db))

	listenAddress := envOrDefault("SERVER_HOST", "0.0.0.0") + ":" + envOrDefault("SERVER_PORT", "8080")
	if err := app.Listen(listenAddress); err != nil {
		log.Fatal(err)
	}
}

// localCORS allows credentialed browser requests from the configured frontend
// origins. Keep the list explicit; wildcard origins cannot use cookies.
func localCORS(c *fiber.Ctx) error {
	origin := c.Get("Origin")
	if origin != "" {
		allowed := os.Getenv("FRONTEND_ORIGIN")
		if allowed == "" {
			allowed = "http://localhost:3000,http://localhost:5173,http://127.0.0.1:3000,http://127.0.0.1:5173"
		}
		for _, value := range strings.Split(allowed, ",") {
			if strings.TrimSpace(value) == origin {
				c.Set("Access-Control-Allow-Origin", origin)
				c.Set("Access-Control-Allow-Credentials", "true")
				c.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
				c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
				c.Set("Vary", "Origin")
				if c.Method() == fiber.MethodOptions {
					return c.SendStatus(fiber.StatusNoContent)
				}
				break
			}
		}
	}
	return c.Next()
}
