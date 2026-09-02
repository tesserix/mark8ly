package carriersecrets_test

import (
	"context"
	"errors"
	"testing"

	secretmanagerclient "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// erroringSMFactory always fails to construct a Secret Manager client,
// exercising the degrade-to-inline path without any network access.
func erroringSMFactory(context.Context) (*secretmanagerclient.Client, error) {
	return nil, errors.New("boom: no secret manager client in tests")
}

// fakeSMFactory returns a non-nil, unusable-but-constructible client so
// mode-selection tests can get past the "client init failed" branch without
// real GCP credentials. It never has AccessLatest/CreateOrAddVersion called
// on it by these tests — they only assert on the returned Store's
// concrete type, not on read/write behaviour.
func fakeSMFactory(ctx context.Context) (*secretmanagerclient.Client, error) {
	return secretmanagerclient.NewClient(ctx,
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithEndpoint("127.0.0.1:1"), // never dialed by these tests
	)
}

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

func TestBuild_GCPSMModeRequiresProjectID(t *testing.T) {
	_, _, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:      "gcpsm",
		Encryptor: crypto.NewNoopEncryptor(),
	})
	if err == nil {
		t.Fatal("Build(gcpsm, no GCPProjectID) returned nil error, want an error")
	}
}

func TestBuild_SMClientFailureDegradesToInline(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         "gcpsm",
		GCPProjectID: "test-project",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewSMClient:  erroringSMFactory,
	})
	if err != nil {
		t.Fatalf("Build(gcpsm, failing SM client) returned an error %v, want degraded fallback instead", err)
	}
	if !degraded {
		t.Error("Build(gcpsm, failing SM client) reported degraded=false, want true")
	}
	if _, ok := store.(*carriersecrets.InlineStore); !ok {
		t.Errorf("Build(gcpsm, failing SM client) returned %T, want *carriersecrets.InlineStore", store)
	}
}

func TestBuild_GCPSMIsNotCached(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         "gcpsm",
		GCPProjectID: "test-project",
		SecretPrefix: "mark8ly-test",
		OpenBaoAddr:  "http://127.0.0.1:8200",
		OpenBaoMount: "kv",
		OpenBaoRole:  "test-role",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewSMClient:  fakeSMFactory,
		NewBaoClient: fakeBaoFactory,
	})
	if err != nil {
		t.Fatalf("Build(gcpsm): %v", err)
	}
	if degraded {
		t.Error("Build(gcpsm) reported degraded=true, want false")
	}
	if _, isCached := store.(*carriersecrets.CachingStore); isCached {
		t.Errorf("Build(gcpsm) returned a *CachingStore — gcpsm MUST stay uncached (see main.go's doc comment on why)")
	}
	if _, isChain := store.(*carriersecrets.ChainStore); !isChain {
		t.Errorf("Build(gcpsm) returned %T, want *carriersecrets.ChainStore", store)
	}
}

func TestBuild_BaoIsCached(t *testing.T) {
	store, degraded, err := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         "bao",
		GCPProjectID: "test-project",
		SecretPrefix: "mark8ly-test",
		OpenBaoAddr:  "http://127.0.0.1:8200",
		OpenBaoMount: "kv",
		OpenBaoRole:  "test-role",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewSMClient:  fakeSMFactory,
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
		GCPProjectID: "test-project",
		SecretPrefix: "mark8ly-test",
		OpenBaoAddr:  "http://127.0.0.1:8200",
		OpenBaoMount: "kv",
		OpenBaoRole:  "test-role",
		Encryptor:    crypto.NewNoopEncryptor(),
		NewSMClient:  fakeSMFactory,
		NewBaoClient: func(bao.Config) (*bao.Client, error) {
			return nil, errors.New("boom: openbao client init failed")
		},
	})
	if err == nil {
		t.Fatal("Build(bao, failing bao client) returned nil error, want an error — never os.Exit, never silent inline")
	}
}
