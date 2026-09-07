package onboarding

import (
	"testing"
)

func TestPromoRateLimiter_AllowsUpToTheCapThenRefuses(t *testing.T) {
	l := NewPromoRateLimiter()

	for i := 0; i < PromoRateMax; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("attempt %d refused, want allowed up to %d", i+1, PromoRateMax)
		}
	}
	if l.Allow("1.2.3.4") {
		t.Fatal("attempt past the cap was allowed")
	}
}

// One caller exhausting its bucket must not lock out another. Worth asserting
// because the buckets share a map, and a keying mistake would present as
// "promo codes stopped working for everyone" only under load.
func TestPromoRateLimiter_BucketsArePerKey(t *testing.T) {
	l := NewPromoRateLimiter()

	for i := 0; i < PromoRateMax; i++ {
		l.Allow("1.2.3.4")
	}
	if !l.Allow("5.6.7.8") {
		t.Fatal("a second caller was refused because the first exhausted its bucket")
	}
}
