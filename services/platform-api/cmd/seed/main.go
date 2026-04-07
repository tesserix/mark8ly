// Command seed loads idempotent reference data into platform-api's database.
//
// Seeds are intentionally separate from migrations: they have different
// lifecycles, change per environment, and need to be re-runnable.
//
// All inserts use ON CONFLICT DO NOTHING so re-running is a no-op for
// existing rows.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	platformapi "github.com/mark8ly/platform-api"
	"github.com/mark8ly/platform-api/internal/location"
	"github.com/mark8ly/platform-api/pkg/db"
)

// locationsFile mirrors seed/locations.json's structure.
type locationsFile struct {
	Countries  []location.Country  `json:"countries"`
	States     []location.State    `json:"states"`
	Currencies []location.Currency `json:"currencies"`
	Timezones  []location.Timezone `json:"timezones"`
}

func main() {
	_ = godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fail("DATABASE_URL is required")
	}

	conn, err := db.Open(dbURL)
	if err != nil {
		fail("db open: %v", err)
	}

	raw, err := platformapi.SeedFS.ReadFile("seed/locations.json")
	if err != nil {
		fail("read seed: %v", err)
	}

	var lf locationsFile
	if err := json.Unmarshal(raw, &lf); err != nil {
		fail("parse seed: %v", err)
	}

	// Order matters: countries → states (FK), then currencies/timezones (independent).
	if len(lf.Countries) > 0 {
		if err := insertIgnoreConflicts(conn, "code", lf.Countries); err != nil {
			fail("seed countries: %v", err)
		}
	}
	if len(lf.Currencies) > 0 {
		if err := insertIgnoreConflicts(conn, "code", lf.Currencies); err != nil {
			fail("seed currencies: %v", err)
		}
	}
	if len(lf.States) > 0 {
		if err := insertIgnoreConflicts(conn, "id", lf.States); err != nil {
			fail("seed states: %v", err)
		}
	}
	if len(lf.Timezones) > 0 {
		if err := insertIgnoreConflicts(conn, "id", lf.Timezones); err != nil {
			fail("seed timezones: %v", err)
		}
	}

	fmt.Printf("seed: %d countries, %d states, %d currencies, %d timezones\n",
		len(lf.Countries), len(lf.States), len(lf.Currencies), len(lf.Timezones))
}

// insertIgnoreConflicts bulk-inserts rows with ON CONFLICT DO NOTHING, keyed
// on the given primary-key column. Idempotent: re-running over an already-seeded
// table is a no-op.
func insertIgnoreConflicts[T any](conn *gorm.DB, pkColumn string, rows []T) error {
	return conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: pkColumn}},
		DoNothing: true,
	}).Create(&rows).Error
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
