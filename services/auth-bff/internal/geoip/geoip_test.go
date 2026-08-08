package geoip

import (
	"net/http"
	"testing"
)

func headers(kv map[string]string) http.Header {
	h := http.Header{}
	for k, v := range kv {
		h.Set(k, v)
	}
	return h
}

func TestCountryFromHeaders(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want string
	}{
		{"cloudflare code", map[string]string{"CF-IPCountry": "IN"}, "IN"},
		{"lowercase is normalised", map[string]string{"CF-IPCountry": "in"}, "IN"},
		{"padded value is trimmed", map[string]string{"CF-IPCountry": " au "}, "AU"},
		{"no headers", nil, ""},
		{"empty value", map[string]string{"CF-IPCountry": ""}, ""},
		{"cloudflare unknown sentinel", map[string]string{"CF-IPCountry": "XX"}, ""},
		{"cloudflare tor sentinel", map[string]string{"CF-IPCountry": "T1"}, ""},
		{"non-alpha2 is rejected", map[string]string{"CF-IPCountry": "INDIA"}, ""},
		{"digits are rejected", map[string]string{"CF-IPCountry": "12"}, ""},
		{"fallback header used when cloudflare absent", map[string]string{"X-Geo-Country": "GB"}, "GB"},
		{"cloudflare wins over fallback", map[string]string{"CF-IPCountry": "IN", "X-Geo-Country": "GB"}, "IN"},
		{"fallback used when cloudflare is a sentinel", map[string]string{"CF-IPCountry": "XX", "X-Geo-Country": "GB"}, "GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountryFromHeaders(headers(tt.in)); got != tt.want {
				t.Errorf("CountryFromHeaders() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountryFromHeadersNilHeader(t *testing.T) {
	if got := CountryFromHeaders(nil); got != "" {
		t.Errorf("CountryFromHeaders(nil) = %q, want empty", got)
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"IN", "India"},
		{"AU", "Australia"},
		{"US", "United States"},
		{"", "an unknown location"},
		{"ZZ", "ZZ"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := Describe(tt.code); got != tt.want {
				t.Errorf("Describe(%q) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}
