// Package migrate wraps golang-migrate for auth-bff.
//
// Intentionally duplicated from services/platform-api/pkg/migrate until a
// third consumer justifies hoisting to a shared Go module.
package migrate

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

type Migrator struct {
	migrationsFS fs.FS
	dbURL        string
}

func New(migrationsFS embed.FS, subdir, dbURL string) (*Migrator, error) {
	sub, err := fs.Sub(migrationsFS, subdir)
	if err != nil {
		return nil, fmt.Errorf("migrate: sub fs: %w", err)
	}
	return &Migrator{migrationsFS: sub, dbURL: dbURL}, nil
}

func (m *Migrator) instance() (*migrate.Migrate, error) {
	src, err := iofs.New(m.migrationsFS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate: iofs source: %w", err)
	}
	mig, err := migrate.NewWithSourceInstance("iofs", src, m.dbURL)
	if err != nil {
		return nil, fmt.Errorf("migrate: new instance: %w", err)
	}
	return mig, nil
}

func (m *Migrator) Up() error {
	mig, err := m.instance()
	if err != nil {
		return err
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: up: %w", err)
	}
	return nil
}

func (m *Migrator) Down(n int) error {
	mig, err := m.instance()
	if err != nil {
		return err
	}
	if err := mig.Steps(-n); err != nil {
		return fmt.Errorf("migrate: down %d: %w", n, err)
	}
	return nil
}

func (m *Migrator) Version() (uint, bool, error) {
	mig, err := m.instance()
	if err != nil {
		return 0, false, err
	}
	v, d, vErr := mig.Version()
	if vErr != nil && !errors.Is(vErr, migrate.ErrNilVersion) {
		return 0, false, vErr
	}
	return v, d, nil
}

func (m *Migrator) AssertVersion(expected uint) error {
	v, dirty, err := m.Version()
	if err != nil {
		return fmt.Errorf("migrate: assert version: %w", err)
	}
	if dirty {
		return fmt.Errorf("migrate: schema is dirty at version %d — manual intervention required", v)
	}
	if v != expected {
		return fmt.Errorf("migrate: schema version %d expected, found %d — run migrations", expected, v)
	}
	return nil
}
