package product_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/product"
)

func TestGenerateHandle_Simple(t *testing.T) {
	if got := product.GenerateHandle("Linen Shirt"); got != "linen-shirt" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateHandle_Unicode(t *testing.T) {
	if got := product.GenerateHandle("Café au lait"); got != "cafe-au-lait" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateHandle_Punctuation(t *testing.T) {
	if got := product.GenerateHandle("Men's T-Shirt & More!"); got != "mens-t-shirt-more" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateHandle_Empty(t *testing.T) {
	if got := product.GenerateHandle(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateHandle_WhitespaceOnly(t *testing.T) {
	if got := product.GenerateHandle("   "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestGenerateHandle_TruncatesToMaxLen(t *testing.T) {
	in := strings.Repeat("a", 300)
	got := product.GenerateHandle(in)
	if len(got) != 200 || got != strings.Repeat("a", 200) {
		t.Fatalf("len=%d got=%q", len(got), got)
	}
}

func TestSuggestNextHandle_BaseFree_ReturnsBase(t *testing.T) {
	got := product.SuggestNextHandle("foo", func(string) bool { return false })
	if got != "foo" {
		t.Fatalf("got %q", got)
	}
}

func TestSuggestNextHandle_BaseTaken_ReturnsBase2(t *testing.T) {
	got := product.SuggestNextHandle("foo", func(c string) bool { return c == "foo" })
	if got != "foo-2" {
		t.Fatalf("got %q", got)
	}
}

func TestSuggestNextHandle_WalksUntilFree(t *testing.T) {
	taken := map[string]bool{"foo": true, "foo-2": true, "foo-3": true}
	got := product.SuggestNextHandle("foo", func(c string) bool { return taken[c] })
	if got != "foo-4" {
		t.Fatalf("got %q", got)
	}
}

func TestSuggestNextHandle_Exhausted_RandomSuffix(t *testing.T) {
	got := product.SuggestNextHandle("foo", func(string) bool { return true })
	if !strings.HasPrefix(got, "foo-") {
		t.Fatalf("no foo- prefix: %q", got)
	}
	// "foo" (3) + "-" (1) + 6 hex chars = 10
	if len(got) != 10 {
		t.Fatalf("want len 10, got %d (%q)", len(got), got)
	}
}
