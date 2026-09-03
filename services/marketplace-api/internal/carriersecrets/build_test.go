package carriersecrets_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// fakeBaoFactory returns a Client without contacting a real OpenBao server
// (Token short-circuits Kubernetes auth per bao.Config's doc comment).
func fakeBaoFactory(cfg bao.Config) (*bao.Client, error) {
	cfg.Token = "fake-test-token"
	return bao.New(cfg)
}

func TestBuild_InlineMode(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:      "inline",
		Encryptor: crypto.NewNoopEncryptor(),
	})
	if err != nil {
		t.Fatalf("Build(inline): %v", err)
	}
	if degraded {
		t.Error("Build(inline) reported degraded=true, want false")
	}
	if _, ok := store.(*carriersecrets.InlineStore); !ok {
		t.Errorf("Build(inline) returned %T, want *carriersecrets.InlineStore", store)
	}
}

func TestBuild_UnknownModeErrors(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:      "not-a-real-mode",
		Encryptor: crypto.NewNoopEncryptor(),
	})
	if err == nil {
		t.Fatal("Build(unknown mode) returned nil error, want an error")
	}
	if store != nil {
		t.Errorf("Build(unknown mode) returned a non-nil store %T, want nil (no silent inline fallback)", store)
	}
	if degraded {
		t.Error("Build(unknown mode) reported degraded=true — an unrecognised mode is an error, not a degrade")
	}
}

// TestBuild_GCPSMModeIsALoudConfigError pins the mark8ly#621 decision:
// "gcpsm" must NEVER be silently coerced to "bao" — a deployment still
// asking for gcpsm believes something false about its own configuration
// (that GCP Secret Manager is still wired), and Build must fail visibly,
// naming the mode, rather than quietly running on a different backend.
func TestBuild_GCPSMModeIsALoudConfigError(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:      "gcpsm",
		Encryptor: crypto.NewNoopEncryptor(),
	})
	if err == nil {
		t.Fatal("Build(gcpsm) returned nil error, want an error — GCP Secret Manager was retired in mark8ly#621")
	}
	if !strings.Contains(err.Error(), "gcpsm") {
		t.Errorf("Build(gcpsm) error = %q, want it to name the mode", err.Error())
	}
	if !strings.Contains(err.Error(), "621") {
		t.Errorf("Build(gcpsm) error = %q, want it to reference mark8ly#621", err.Error())
	}
	if store != nil {
		t.Errorf("Build(gcpsm) returned a non-nil store %T, want nil", store)
	}
	if degraded {
		t.Error("Build(gcpsm) reported degraded=true, want false — this is a hard config error, not a degrade")
	}
}

func TestBuild_BaoIsCached(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         "bao",
		OpenBaoAddr:  "http://127.0.0.1:8200",
		OpenBaoMount: "kv",
		OpenBaoRole:  "test-role",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewBaoClient: fakeBaoFactory,
	})
	if err != nil {
		t.Fatalf("Build(bao): %v", err)
	}
	if degraded {
		t.Error("Build(bao) reported degraded=true, want false")
	}
	if _, ok := store.(*carriersecrets.CachingStore); !ok {
		t.Errorf("Build(bao) returned %T, want *carriersecrets.CachingStore", store)
	}
}

func TestBuild_BaoClientFailureIsAnError(t *testing.T) {
	_, _, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         "bao",
		OpenBaoAddr:  "http://127.0.0.1:8200",
		OpenBaoMount: "kv",
		OpenBaoRole:  "test-role",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewBaoClient: func(bao.Config) (*bao.Client, error) {
			return nil, errors.New("boom: openbao client init failed")
		},
	})
	if err == nil {
		t.Fatal("Build(bao, failing bao client) returned nil error, want an error — never os.Exit, never silent inline")
	}
}
