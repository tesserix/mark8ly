// Package db opens a GORM connection to Postgres with retry.
package db

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open dials Postgres with exponential-ish backoff (10 attempts).
// Returns the GORM DB or the last error.
func Open(dsn string) (*gorm.DB, error) {
	var db *gorm.DB
	var err error
	for attempt := 1; attempt <= 10; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return nil, fmt.Errorf("db: open after 10 attempts: %w", err)
}
