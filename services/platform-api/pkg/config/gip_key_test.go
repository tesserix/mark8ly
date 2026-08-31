package config

import "testing"

// TestGIPKey covers the choice between the server key and the public web
// key.
//
// The distinction is not cosmetic: the web key is the value the admin
// browser bundle embeds, so it carries an HTTP-referrer restriction. A
// server sends no Referer, and GIP answers referrer-restricted admin calls
// such as resetPassword with 403 "Requests from referer <empty> are
// blocked". Preferring the server key is what makes password reset work.
func TestGIPKey(t *testing.T) {
	tests := []struct {
		name   string
		server string
		web    string
		want   string
	}{
		{
			name:   "server key wins when both are set",
			server: "server-key",
			web:    "web-key",
			want:   "server-key",
		},
		{
			// The fallback exists so this change can ship before the
			// server key is provisioned; such a deployment behaves
			// exactly as it did before.
			name: "falls back to the web key when no server key is set",
			web:  "web-key",
			want: "web-key",
		},
		{
			name:   "server key is used even when the web key is absent",
			server: "server-key",
			want:   "server-key",
		},
		{
			name: "empty when neither is set, so the caller skips wiring GIP",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{GIPServerAPIKey: tc.server, GIPWebAPIKey: tc.web}
			if got := c.GIPKey(); got != tc.want {
				t.Fatalf("GIPKey() = %q, want %q", got, tc.want)
			}
		})
	}
}
