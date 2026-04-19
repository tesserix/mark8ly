package cron

import (
	"github.com/google/uuid"
)

// parseUUID is a small helper that wraps uuid.Parse to keep call sites clean.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
