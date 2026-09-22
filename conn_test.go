package main

import (
	"strings"
	"testing"
)

func TestDatabaseConfiguration(t *testing.T) {
	before := dotenv
	t.Cleanup(func() { dotenv = before })
	for _, password := range []string{"", " \t"} {
		dotenv = map[string]string{"DB_PASSWORD": password}
		if _, err := databaseConnectionString(); err == nil {
			t.Fatal("missing configuration accepted")
		}
	}
	dotenv = map[string]string{"DATABASE_URL": "postgres://example/test-only"}
	got, err := databaseConnectionString()
	if err != nil || got != dotenv["DATABASE_URL"] {
		t.Fatal("explicit DATABASE_URL was not preserved")
	}
	dotenv = map[string]string{"DB_PASSWORD": "test ' password\\value"}
	got, err = databaseConnectionString()
	if err != nil || !strings.Contains(got, "password='test \\' password\\\\value'") {
		t.Fatal("lib/pq password quoting failed")
	}
}
