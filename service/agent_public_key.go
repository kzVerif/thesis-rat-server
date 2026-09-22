package service

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"strings"
)

// Bound the input before trimming/decoding; only outer whitespace is allowed.
const maxPublicKeyInputBytes = 256

func canonicalAgentPublicKey(input string) (string, error) {
	invalid := errors.New("public_key must be a standard padded Base64 Ed25519 public key")
	if len(input) > maxPublicKeyInputBytes {
		return "", invalid
	}
	value := strings.TrimSpace(input)
	if len(value) != base64.StdEncoding.EncodedLen(ed25519.PublicKeySize) ||
		strings.ContainsAny(value, "\r\n\t ") {
		return "", invalid
	}
	key, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return "", invalid
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
