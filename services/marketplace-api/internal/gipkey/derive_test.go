package gipkey

import (
	"reflect"
	"testing"
)

func TestDeriveReferrers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "apex",
			in:   "primasyss.com",
			want: []string{"https://primasyss.com/*", "https://*.primasyss.com/*"},
		},
		{
			name: "uppercase trimmed and lowered",
			in:   "  PrimaSyss.COM  ",
			want: []string{"https://primasyss.com/*", "https://*.primasyss.com/*"},
		},
		{
			name: "subdomain custom domain",
			in:   "shop.acme.co",
			want: []string{"https://shop.acme.co/*", "https://*.shop.acme.co/*"},
		},
		{
			name: "trailing dot stripped",
			in:   "example.com.",
			want: []string{"https://example.com/*", "https://*.example.com/*"},
		},
		{
			name: "empty rejected",
			in:   "",
			want: nil,
		},
		{
			name: "single label rejected",
			in:   "localhost",
			want: nil,
		},
		{
			name: "url-shaped input rejected",
			in:   "https://primasyss.com/",
			want: nil,
		},
		{
			name: "path in input rejected",
			in:   "primasyss.com/sign-in",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveReferrers(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("DeriveReferrers(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
