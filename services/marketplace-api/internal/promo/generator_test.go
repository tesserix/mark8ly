package promo

import (
	"strings"
	"testing"
)

func TestGenerateCode_LengthBounds(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{"below minimum", 11, true},
		{"at minimum", 12, false},
		{"above minimum", 16, false},
		{"large", 32, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GenerateCode(tc.length)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GenerateCode(%d) error = %v, wantErr %v", tc.length, err, tc.wantErr)
			}
			if !tc.wantErr && len(got) != tc.length {
				t.Errorf("GenerateCode(%d) returned length %d, want %d", tc.length, len(got), tc.length)
			}
		})
	}
}

func TestGenerateCode_CharsetInvariants(t *testing.T) {
	const iterations = 500
	forbidden := "0O1lI"

	for i := 0; i < iterations; i++ {
		code, err := GenerateCode(MinCodeLength)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ch := range code {
			if strings.ContainsRune(forbidden, ch) {
				t.Errorf("code %q contains forbidden character %q", code, ch)
			}
			if !strings.ContainsRune(charset, ch) {
				t.Errorf("code %q contains character %q not in charset", code, ch)
			}
		}
	}
}

func TestGenerateCode_NoCollisionsOver10k(t *testing.T) {
	const n = 10_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		code, err := GenerateCode(MinCodeLength)
		if err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
		if _, dup := seen[code]; dup {
			t.Fatalf("collision detected at iteration %d: %q", i, code)
		}
		seen[code] = struct{}{}
	}
}
