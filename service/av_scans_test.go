package service

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestAVScanRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	app := fiber.New()
	app.Get("/scans", ListAVScanResults(nil))
	app.Get("/scans/:id", GetAVScanResult(nil))
	for _, path := range []string{
		"/scans?page=0", "/scans?page=-1", "/scans?page=abc",
		"/scans?limit=0", "/scans?limit=101", "/scans?limit=1.5",
		"/scans?page=9223372036854775807&limit=100",
		"/scans?agent_id=invalid", "/scans?job_id=invalid",
		"/scans?command_id=invalid", "/scans?status=UNKNOWN", "/scans/invalid",
	} {
		t.Run(path, func(t *testing.T) {
			response, err := app.Test(httptest.NewRequest("GET", path, nil))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != 400 {
				t.Fatalf("status = %d, want 400", response.StatusCode)
			}
		})
	}
}
