package emailtemplates

import (
	"sync"
	"testing"
)

// RegisteredKeys is the only view of the in-memory registry, and it is
// what makes a registered-but-unseeded key visible at all (mark8ly#717).
func TestRegisteredKeysReturnsEveryRegisteredKeySorted(t *testing.T) {
	l := NewLoader(nil)
	l.Register("win_back_day30", EmbeddedFallback{Subject: "c"})
	l.Register("dunning_day_5", EmbeddedFallback{Subject: "a"})
	l.Register("orderdoc_invoice", EmbeddedFallback{Subject: "b"})

	got := l.RegisteredKeys()
	want := []string{"dunning_day_5", "orderdoc_invoice", "win_back_day30"}
	if len(got) != len(want) {
		t.Fatalf("RegisteredKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RegisteredKeys() = %v, want %v (sorted)", got, want)
		}
	}
}

func TestRegisteredKeysOnEmptyRegistry(t *testing.T) {
	if got := NewLoader(nil).RegisteredKeys(); len(got) != 0 {
		t.Fatalf("RegisteredKeys() = %v, want empty", got)
	}
}

// The nil receiver is safe because a handler may hold a nil registry —
// the same tolerance every other exported Loader method has.
func TestRegistryAccessorsAreNilReceiverSafe(t *testing.T) {
	var l *Loader
	if got := l.RegisteredKeys(); got != nil {
		t.Fatalf("RegisteredKeys() on nil loader = %v, want nil", got)
	}
	if _, ok := l.Fallback("anything"); ok {
		t.Fatal("Fallback() on nil loader reported a hit")
	}
}

// The second result must distinguish "registered with empty copy" from
// "not registered". A caller that treated the zero value as an empty
// template would let an operator author a key no call site renders.
func TestFallbackReportsRegistrationSeparatelyFromContent(t *testing.T) {
	l := NewLoader(nil)
	l.Register("blank", EmbeddedFallback{})

	fb, ok := l.Fallback("blank")
	if !ok {
		t.Fatal("Fallback(blank) = false, want true — the key IS registered")
	}
	if fb.Subject != "" {
		t.Fatalf("Fallback(blank).Subject = %q, want empty", fb.Subject)
	}
	if _, ok := l.Fallback("never_registered"); ok {
		t.Fatal("Fallback(never_registered) = true, want false")
	}
}

// Registration happens during boot while sends may already be in flight,
// so the accessors take the same RWMutex Register and Render do. Run with
// -race.
func TestRegistryAccessorsAreRaceFree(t *testing.T) {
	l := NewLoader(nil)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); l.Register("k", EmbeddedFallback{Subject: "s"}) }()
		go func() {
			defer wg.Done()
			_ = l.RegisteredKeys()
			_, _ = l.Fallback("k")
		}()
	}
	wg.Wait()
}
