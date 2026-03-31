package db

import (
    "database/sql"
    "fmt"
    "log"
    "os"

    _ "github.com/lib/pq"
)

var DB *sql.DB

func Init() error {
    connStr := os.Getenv("DATABASE_URL")
    if connStr == "" {
        // Fallback for local development
       connStr = "postgres://postgres@localhost:5432/plastic?sslmode=disable"
    }

    var err error
    DB, err = sql.Open("postgres", connStr)
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }

    err = DB.Ping()
    if err != nil {
        return fmt.Errorf("failed to ping database: %w", err)
    }

    log.Println("Database connected")
    return nil
}
