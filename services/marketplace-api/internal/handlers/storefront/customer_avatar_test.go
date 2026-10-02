package storefront

import "testing"

// A shopper could set avatar_url to any https://storage.googleapis.com/
// object — the old check tested nothing but that prefix. Nothing here
// mints a customer avatar key, so every non-empty value names an object
// the shopper does not own, including a merchant's product image in our
// own public media bucket.
//
// These cases are the hole, written as the attack rather than as a
// paraphrase of the implementation.
func TestAcceptAvatarURLUpdate_RefusesAnyObjectTheShopperNames(t *testing.T) {
	hostile := map[string]string{
		"a merchant's product image in our own bucket": "https://storage.googleapis.com/mark8ly-prod-media/tenants/t1/products/media/abc/hero.jpg",
		"a staff member's avatar":                      "https://storage.googleapis.com/mark8ly-prod-media/users/u1/avatar/9f1.png",
		"an object in someone else's bucket":           "https://storage.googleapis.com/another-bucket/whatever.png",
		"an arbitrary external URL":                    "https://evil.example.com/tracker.gif",
		"a plausible-looking own-bucket key":           "https://storage.googleapis.com/mark8ly-prod-media/customers/c1/avatar/x.png",
		"whitespace-padded":                            "   https://storage.googleapis.com/mark8ly-prod-media/tenants/t1/x.jpg   ",
	}
	for name, url := range hostile {
		t.Run(name, func(t *testing.T) {
			if _, ok := acceptAvatarURLUpdate(url); ok {
				t.Fatalf("accepted an avatar the shopper does not own: %q", url)
			}
		})
	}
}

// Clearing has to keep working, or a customer who already has one of
// these set can never get rid of it.
func TestAcceptAvatarURLUpdate_AllowsClearing(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t\n"} {
		got, ok := acceptAvatarURLUpdate(raw)
		if !ok {
			t.Fatalf("clearing must be allowed, rejected %q", raw)
		}
		if got != "" {
			t.Fatalf("clearing must normalise to empty, got %q", got)
		}
	}
}

// The last case in the hostile table is the one to re-read when #971's
// upload endpoint lands: a key under customers/<id>/ is refused TODAY
// because nothing mints it, not because the shape is wrong. Whoever adds
// the endpoint has to widen this deliberately.
func TestAcceptAvatarURLUpdate_OwnBucketIsNotEnoughOnItsOwn(t *testing.T) {
	ours := "https://storage.googleapis.com/mark8ly-prod-media/customers/c1/avatar/x.png"
	if _, ok := acceptAvatarURLUpdate(ours); ok {
		t.Fatal("a customers/ key is still not acceptable while nothing mints one")
	}
}
