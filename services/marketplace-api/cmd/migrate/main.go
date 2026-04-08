// Command migrate runs database migrations for platform-api.
//
// Usage:
//
//	migrate up                   apply all pending migrations
//	migrate down N               roll back N migrations
//	migrate version              print current schema version
//
// This is a separate binary from cmd/server. It runs as an init container in
// docker-compose, as a CI step before deploy, and as a Helm hook before the
// app rolls out.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	marketplaceapi "github.com/mark8ly/marketplace-api"
	"github.com/mark8ly/marketplace-api/pkg/migrate"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	_ = godotenv.Load()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fail("DATABASE_URL is required")
	}
	mig, err := migrate.New(marketplaceapi.MigrationsFS, "migrations", dbURL)
	if err != nil {
		fail("migrate init: %v", err)
	}

	switch os.Args[1] {
	case "up":
		if err := mig.Up(); err != nil {
			fail("up: %v", err)
		}
		fmt.Println("migrations applied")
	case "down":
		n := 1
		if len(os.Args) >= 3 {
			parsed, err := strconv.Atoi(os.Args[2])
			if err != nil || parsed < 1 {
				fail("down: invalid step count %q", os.Args[2])
			}
			n = parsed
		}
		if err := mig.Down(n); err != nil {
			fail("down: %v", err)
		}
		fmt.Printf("rolled back %d migration(s)\n", n)
	case "version":
		v, dirty, err := mig.Version()
		if err != nil {
			fail("version: %v", err)
		}
		fmt.Printf("version=%d dirty=%v\n", v, dirty)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate <up|down [N]|version>")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
