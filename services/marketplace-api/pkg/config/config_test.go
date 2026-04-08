package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	os.Unsetenv("DATABASE_URL")
	_, err := Load()
	if err == nil {
		t.Fatal("Load() with no DATABASE_URL = nil, want error")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://x/y")
	defer os.Unsetenv("DATABASE_URL")
	os.Unsetenv("ENV")
	os.Unsetenv("MODE")
	os.Unsetenv("HTTP_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want dev", cfg.Env)
	}
	if cfg.Mode != "both" {
		t.Errorf("Mode = %q, want both", cfg.Mode)
	}
	if cfg.HTTPPort != 8087 {
		t.Errorf("HTTPPort = %d, want 8087", cfg.HTTPPort)
	}
}
