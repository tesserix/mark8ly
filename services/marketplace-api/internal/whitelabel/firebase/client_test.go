package firebase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/firebase"
)

func TestClient_ReturnsNotWired(t *testing.T) {
	cli := firebase.New()
	if err := cli.ArchiveProject(context.Background(), "fb-proj"); !errors.Is(err, firebase.ErrNotWired) {
		t.Errorf("ArchiveProject = %v, want wraps ErrNotWired", err)
	}
	if err := cli.DeleteProject(context.Background(), "fb-proj"); !errors.Is(err, firebase.ErrNotWired) {
		t.Errorf("DeleteProject = %v, want wraps ErrNotWired", err)
	}
}

func TestFakeClient_Counts(t *testing.T) {
	f := firebase.NewFakeClient()
	_ = f.ArchiveProject(context.Background(), "a")
	_ = f.ArchiveProject(context.Background(), "b")
	_ = f.DeleteProject(context.Background(), "a")
	if f.ArchiveProjectCalls != 2 {
		t.Errorf("Archive calls = %d, want 2", f.ArchiveProjectCalls)
	}
	if f.DeleteProjectCallCount != 1 {
		t.Errorf("Delete calls = %d, want 1", f.DeleteProjectCallCount)
	}
}
