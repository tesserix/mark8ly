package webhook

import (
	"strings"
	"testing"
	"time"
)

func TestSign_IsStableForTheSameInputs(t *testing.T) {
	ts := time.Unix(1756800000, 0)
	a := Sign("shh", ts, []byte(`{"event":"order.placed"}`))
	b := Sign("shh", ts, []byte(`{"event":"order.placed"}`))
	if a != b {
		t.Fatalf("signature not deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "t=1756800000,v1=") {
		t.Fatalf("unexpected format: %q", a)
	}
}

// The timestamp must be INSIDE the signed material. If it were only a
// separate header, a captured delivery could be replayed later with a fresh
// timestamp and still verify.
func TestSign_ChangesWithTimestamp(t *testing.T) {
	body := []byte(`{"event":"order.placed"}`)
	a := Sign("shh", time.Unix(1756800000, 0), body)
	b := Sign("shh", time.Unix(1756800001, 0), body)
	if a == b {
		t.Fatal("signature must cover the timestamp")
	}
}

func TestSign_ChangesWithBodyAndSecret(t *testing.T) {
	ts := time.Unix(1756800000, 0)
	base := Sign("shh", ts, []byte(`{"a":1}`))
	if Sign("shh", ts, []byte(`{"a":2}`)) == base {
		t.Fatal("signature must cover the body")
	}
	if Sign("other", ts, []byte(`{"a":1}`)) == base {
		t.Fatal("signature must depend on the secret")
	}
}

func TestGenerateSecret_IsRandomAndLongEnough(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s, err := GenerateSecret()
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 64 {
			t.Fatalf("want 64 hex chars, got %d", len(s))
		}
		if seen[s] {
			t.Fatal("duplicate secret generated")
		}
		seen[s] = true
	}
}
