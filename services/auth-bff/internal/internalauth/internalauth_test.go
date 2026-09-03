package internalauth

import "testing"

func TestEqual(t *testing.T) {
	const want = "s3cret-internal"
	cases := []struct {
		name string
		got  string
		want string
		ok   bool
	}{
		{"exact match", want, want, true},
		{"missing header", "", want, false},
		{"wrong value", "nope", want, false},
		{"prefix of the secret", want[:4], want, false},
		{"unconfigured expectation never matches", want, "", false},
		{"both empty", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Equal(tc.got, tc.want); got != tc.ok {
				t.Fatalf("Equal(%q, %q) = %v, want %v", tc.got, tc.want, got, tc.ok)
			}
		})
	}
}

func TestHeaderNameMatchesTheEstablishedScheme(t *testing.T) {
	if Header != "X-Internal-Auth" {
		t.Fatalf("Header = %q; internal/audit and internal/notify send X-Internal-Auth", Header)
	}
}
