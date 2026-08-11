package main

import (
	"database/sql"
	"log"
)

var db *sql.DB

func SetupDatabase() *sql.DB {
	const connectionString = "user=postgres password=kanghunz12 dbname=ratsystem sslmode=disable"

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
