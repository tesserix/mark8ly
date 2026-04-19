package main

import (
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
)

func TestShouldRotate_NewestOver30d(t *testing.T) {
	vs := []arbitrage.KeyVersion{
		{Name: "v1", Payload: []byte("k"), CreatedAt: time.Now().Add(-31 * 24 * time.Hour)},
	}
	if !shouldRotate(vs) {
		t.Error("expected shouldRotate=true for newest version aged 31d")
	}
}

func TestShouldRotate_NewestUnder30d(t *testing.T) {
	vs := []arbitrage.KeyVersion{
		{Name: "v1", Payload: []byte("k"), CreatedAt: time.Now().Add(-5 * 24 * time.Hour)},
	}
	if shouldRotate(vs) {
		t.Error("expected shouldRotate=false for newest version aged 5d")
	}
}

func TestShouldRotate_EmptyForcesFirstWrite(t *testing.T) {
	if !shouldRotate(nil) {
		t.Error("expected shouldRotate=true for empty versions (bootstrap)")
	}
}

func TestVersionsToDisable(t *testing.T) {
	now := time.Now()
	vs := []arbitrage.KeyVersion{
		{Name: "v1", CreatedAt: now.Add(-10 * 24 * time.Hour)},  // 10d — keep
		{Name: "v2", CreatedAt: now.Add(-40 * 24 * time.Hour)},  // 40d — keep
		{Name: "v3", CreatedAt: now.Add(-62 * 24 * time.Hour)},  // 62d — disable
		{Name: "v4", CreatedAt: now.Add(-100 * 24 * time.Hour)}, // 100d — disable
	}
	toDisable := versionsToDisable(vs, 61)
	if len(toDisable) != 2 {
		t.Fatalf("len(toDisable)=%d, want 2", len(toDisable))
	}
	names := map[string]bool{}
	for _, v := range toDisable {
		names[v.Name] = true
	}
	if !names["v3"] || !names["v4"] {
		t.Errorf("expected v3 and v4 to be disabled, got %v", names)
	}
}

func TestVersionsToDisable_NoneOld(t *testing.T) {
	now := time.Now()
	vs := []arbitrage.KeyVersion{
		{Name: "v1", CreatedAt: now.Add(-5 * 24 * time.Hour)},
		{Name: "v2", CreatedAt: now.Add(-29 * 24 * time.Hour)},
	}
	toDisable := versionsToDisable(vs, 61)
	if len(toDisable) != 0 {
		t.Errorf("expected 0 versions to disable, got %d", len(toDisable))
	}
}
