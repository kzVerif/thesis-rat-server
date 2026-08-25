package main

import (
	"rat-server/service"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	_ "github.com/lib/pq"
)

func main() {
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000, http://localhost:5173, http://127.0.0.1:3000, http://127.0.0.1:5173",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
		AllowCredentials: true,
		AllowMethods:     "GET, POST, PUT, DELETE, OPTIONS",
	}))

	db = SetupDatabase()
	defer db.Close()

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
	roles := api.Group("/roles")
	roles.Get("/", service.ListRoles(db))
	roles.Get("/:id", service.GetRole(db))
	roles.Post("/", service.RequireAdministrator, service.CreateRole(db))
	roles.Put("/:id", service.RequireAdministrator, service.UpdateRole(db))
	roles.Delete("/:id", service.RequireAdministrator, service.DeleteRole(db))

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
	tokens := api.Group("/tokens", service.RequirePermission(db, service.UsersManagePermission))
	tokens.Get("/", service.ListTokens(db))
	tokens.Post("/", service.CreateToken(db))
	tokens.Put("/:id", service.UpdateToken(db))
	tokens.Patch("/:id", service.UpdateToken(db))
	tokens.Delete("/:id", service.RevokeToken(db))
	tokens.Post("/validate", service.ValidateToken(db))

	app.Listen(":8080")
}
