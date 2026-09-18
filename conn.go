package main

import (
	"database/sql"
	"fmt"
	"log"
)

var db *sql.DB

func SetupDatabase() *sql.DB {
	connectionString := envOrDefault("DATABASE_URL", "")
	if connectionString == "" {
		connectionString = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			envOrDefault("DB_HOST", "localhost"),
			envOrDefault("DB_PORT", "5432"),
			envOrDefault("DB_USER", "postgres"),
			envOrDefault("DB_PASSWORD", "102247"),
			envOrDefault("DB_NAME", "ratsystem"),
			envOrDefault("DB_SSLMODE", "disable"),
		)
	}

	var err error
	db, err = sql.Open("postgres", connectionString)
	if err != nil {
		log.Fatal(err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	return db
}
