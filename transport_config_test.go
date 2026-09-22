package main

import (
	"github.com/gofiber/fiber/v2"
	"net/http/httptest"
	"testing"
)

func TestProductionTransportConfig(t *testing.T) {
	before := dotenv
	t.Cleanup(func() { dotenv = before })
	for _, tc := range []struct {
		name, host, origins, mode string
		valid                     bool
	}{
		{"valid", "127.0.0.1", "https://lab.example.com", "production", true},
		{"IPv6", "::1", "https://lab.example.com", "production", true},
		{"public origin bind", "0.0.0.0", "https://lab.example.com", "production", false},
		{"missing origins", "127.0.0.1", "", "production", false},
		{"wildcard", "127.0.0.1", "https://*.example.com", "production", false},
		{"plaintext browser", "127.0.0.1", "http://lab.example.com", "production", false},
		{"userinfo", "127.0.0.1", "https://secret@lab.example.com", "production", false},
		{"path", "127.0.0.1", "https://lab.example.com/path", "production", false},
		{"development", "0.0.0.0", "http://localhost:3000", "development", true},
		{"unknown mode", "127.0.0.1", "https://lab.example.com", "prod", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dotenv = map[string]string{"TRANSPORT_MODE": tc.mode, "SERVER_HOST": tc.host, "FRONTEND_ORIGIN": tc.origins}
			if err := validateTransportConfig(); (err == nil) != tc.valid {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}

func TestCORSAndForwardedPeerBoundary(t *testing.T) {
	before := dotenv
	t.Cleanup(func() { dotenv = before })
	dotenv = map[string]string{"FRONTEND_ORIGIN": "https://lab.example.com"}
	app := fiber.New()
	app.Use(localCORS)
	app.Get("/", func(c *fiber.Ctx) error { return c.SendString(c.IP()) })
	for _, origin := range []string{"https://lab.example.com", "https://evil.example.com"} {
		req := httptest.NewRequest("GET", "http://localhost/", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("X-Forwarded-For", "198.51.100.123")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if (resp.Header.Get("Access-Control-Allow-Origin") != "") != (origin == "https://lab.example.com") {
			t.Fatal("CORS boundary changed")
		}
	}
	// No proxy header is configured; Fiber must retain its direct-peer model.
	if app.Config().ProxyHeader != "" {
		t.Fatal("untrusted forwarding headers enabled")
	}
}
