package googleplay_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
)

func TestNew_RequiresCredsFetcher(t *testing.T) {
	if _, err := googleplay.New(googleplay.Config{}); err == nil {
		t.Error("New(empty) = nil, want error")
	}
}

func TestClient_BlockDownloads_ReturnsNotWired(t *testing.T) {
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{ServiceAccountJSON: []byte(`{"type":"service_account"}`)}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = cli.BlockDownloads(context.Background(), "com.example.app")
	if !errors.Is(err, googleplay.ErrNotWired) {
		t.Errorf("BlockDownloads = %v, want wraps ErrNotWired", err)
	}
}

func TestClient_PullApp_ReturnsNotWired(t *testing.T) {
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{ServiceAccountJSON: []byte(`{"type":"service_account"}`)}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := cli.PullApp(context.Background(), "com.example.app"); !errors.Is(err, googleplay.ErrNotWired) {
		t.Errorf("PullApp = %v, want wraps ErrNotWired", err)
	}
}

func TestClient_CredsFetcherError_SurfacesBeforeNotWired(t *testing.T) {
	sentinel := errors.New("creds read failed")
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{}, sentinel
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = cli.BlockDownloads(context.Background(), "com.example")
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wraps sentinel (creds failure should surface before ErrNotWired)", err)
	}
}

func TestFakeClient_Counts(t *testing.T) {
	f := googleplay.NewFakeClient()
	_ = f.BlockDownloads(context.Background(), "com.a")
	_ = f.PullApp(context.Background(), "com.a")
	_ = f.PullApp(context.Background(), "com.b")
	if f.BlockDownloadsCallCount != 1 {
		t.Errorf("Block count = %d, want 1", f.BlockDownloadsCallCount)
	}
	if f.PullAppCallCount != 2 {
		t.Errorf("Pull count = %d, want 2", f.PullAppCallCount)
	}
}
