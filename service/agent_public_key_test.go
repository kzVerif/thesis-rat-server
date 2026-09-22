package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func TestCanonicalAgentPublicKey(t *testing.T) {
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	valid := base64.StdEncoding.EncodeToString(pub)
	for _, value := range []string{valid, " \r\n\t" + valid + " \n"} {
		got, err := canonicalAgentPublicKey(value)
		if err != nil || got != valid {
			t.Fatalf("valid public key rejected or changed: %v", err)
		}
	}
	// The final sextet has two zero padding bits; setting one must be rejected.
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	noncanonical := valid[:42] + string(alphabet[strings.IndexByte(alphabet, valid[42])+1]) + "="
	for name, value := range map[string]string{
		"empty": "", "whitespace": " \t", "invalid": strings.Repeat("!", 44),
		"short":         base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize-1)),
		"long":          base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize+1)),
		"private":       base64.StdEncoding.EncodeToString(private),
		"seed-with-PEM": "-----BEGIN PRIVATE KEY-----\n" + valid,
		"unpadded":      strings.TrimRight(valid, "="),
		"url-alphabet":  strings.Repeat("_", 43) + "=",
		"wrapped":       valid[:10] + "\n" + valid[10:],
		"oversized":     strings.Repeat(" ", maxPublicKeyInputBytes) + valid,
		"padding-bits":  noncanonical,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := canonicalAgentPublicKey(value); err == nil {
				t.Fatal("malformed public key accepted")
			}
			// A nil DB proves rejection happens before token consumption or SQL.
			app := fiber.New()
			app.Post("/register", RegisterAgent(nil))
			body, _ := json.Marshal(map[string]string{
				"token": "test-only", "agent_id": uuid.NewString(), "hostname": "test",
				"mac_address": "aa:bb:cc:dd:ee:ff", "public_key": value,
			})
			req := httptest.NewRequest("POST", "/register", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 400 {
				t.Fatalf("status=%d", res.StatusCode)
			}
		})
	}
}

func TestRegistrationRejectsPrivateKeyFields(t *testing.T) {
	for _, field := range []string{"private_key", "encrypted_private_key"} {
		t.Run(field, func(t *testing.T) {
			app := fiber.New()
			app.Post("/register", RegisterAgent(nil))
			body, _ := json.Marshal(map[string]string{
				"token": "test-only", "agent_id": uuid.NewString(), "hostname": "test",
				"mac_address": "aa:bb:cc:dd:ee:ff",
				"public_key":  base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)),
				field:         "must-not-be-accepted",
			})
			req := httptest.NewRequest("POST", "/register", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != 400 {
				t.Fatalf("status=%d", res.StatusCode)
			}
		})
	}
}
