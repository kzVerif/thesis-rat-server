package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Go remains an HTTP origin behind a same-host TLS proxy/tunnel.
// Forwarding headers are not trusted for authentication, cookies or client IP.
func validateTransportConfig() error {
	mode := envOrDefault("TRANSPORT_MODE", "development")
	if mode != "development" && mode != "production" {
		return fmt.Errorf("TRANSPORT_MODE must be production or development")
	}
	if mode != "production" {
		return nil
	}
	ip := net.ParseIP(envOrDefault("SERVER_HOST", "0.0.0.0"))
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("production SERVER_HOST must be a loopback IP behind a same-host HTTPS proxy/tunnel")
	}
	origins := strings.TrimSpace(dotenv["FRONTEND_ORIGIN"])
	if origins == "" {
		return fmt.Errorf("production FRONTEND_ORIGIN must explicitly list trusted HTTPS origins")
	}
	for _, origin := range strings.Split(origins, ",") {
		u, err := url.Parse(strings.TrimSpace(origin))
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(u.Host, "*?\\") {
			return fmt.Errorf("production FRONTEND_ORIGIN requires exact HTTPS origins without wildcard, credentials, path or query")
		}
	}
	return nil
}
