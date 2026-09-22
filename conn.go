package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

var db *sql.DB

// Retain this server's .env-only configuration policy; never supply a password.
func databaseConnectionString() (string, error) {
	if value := strings.TrimSpace(envOrDefault("DATABASE_URL", "")); value != "" {
		return value, nil
	}
	password := envOrDefault("DB_PASSWORD", "")
	if strings.TrimSpace(password) == "" {
		return "", fmt.Errorf("configure DATABASE_URL or DB_PASSWORD in .env")
	}
	// Quote lib/pq values so spaces, quotes and backslashes remain literal.
	quote := func(value string) string {
		return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(value) + "'"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		quote(envOrDefault("DB_HOST", "localhost")),
		quote(envOrDefault("DB_PORT", "5432")),
		quote(envOrDefault("DB_USER", "postgres")),
		quote(password),
		quote(envOrDefault("DB_NAME", "ratsystem")),
		quote(envOrDefault("DB_SSLMODE", "disable"))), nil
}

func SetupDatabase() *sql.DB {
	connectionString, err := databaseConnectionString()
	if err != nil {
		log.Fatal(err)
	}
	db, err = sql.Open("postgres", connectionString)
	if err != nil {
		log.Fatal("database configuration is invalid; check .env")
	}
	if err = db.Ping(); err != nil {
		log.Fatal("database connection failed; check configuration and database availability")
	}
	return db
}
