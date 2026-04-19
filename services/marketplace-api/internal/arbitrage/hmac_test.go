package arbitrage

import (
	"context"
	"testing"
	"time"
)

func makeHasherWithKey(t *testing.T, key []byte, name string) *Hasher {
	t.Helper()
	src := &stubSource{
		versions: []KeyVersion{{Name: name, Payload: key, CreatedAt: time.Now()}},
	}
	loader := NewKeyLoader(src, 5*time.Minute)
	return NewHasher(loader)
}

func TestHasher_DeterministicUnderSameKey(t *testing.T) {
	key := []byte("test-key-32-bytes-exactly-padded")
	h := makeHasherWithKey(t, key, "v1")

	r1, err := h.Hash(context.Background(), "203.0.113.1")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	r2, err := h.Hash(context.Background(), "203.0.113.1")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if r1.Hex != r2.Hex {
		t.Errorf("hashes differ under same key: %q vs %q", r1.Hex, r2.Hex)
	}
	if len(r1.Hex) != 64 {
		t.Errorf("len(Hex)=%d, want 64", len(r1.Hex))
	}
}

func TestHasher_DifferentKeysProduceDifferentDigests(t *testing.T) {
	keyA := []byte("key-A-32-bytes-exactly-padded!!!")
	keyB := []byte("key-B-32-bytes-exactly-padded!!!")
	hA := makeHasherWithKey(t, keyA, "vA")
	hB := makeHasherWithKey(t, keyB, "vB")

	rA, _ := hA.Hash(context.Background(), "203.0.113.1")
	rB, _ := hB.Hash(context.Background(), "203.0.113.1")
	if rA.Hex == rB.Hex {
		t.Error("same IP hashed under different keys produced identical digest — rotation does not sever correlation")
	}
}

func TestHasher_RecordsKeyVersion(t *testing.T) {
	h := makeHasherWithKey(t, []byte("some-key"), "projects/proj/secrets/s/versions/3")
	r, err := h.Hash(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if r.KeyVersion != "projects/proj/secrets/s/versions/3" {
		t.Errorf("KeyVersion=%q, want the Secret Manager resource name", r.KeyVersion)
	}
}

func TestHasher_EmptyIPReturnsEmpty(t *testing.T) {
	h := makeHasherWithKey(t, []byte("some-key"), "v1")
	r, err := h.Hash(context.Background(), "")
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if r.Hex != "" || r.KeyVersion != "" {
		t.Errorf("expected empty HashResult for empty IP, got %+v", r)
	}
}
