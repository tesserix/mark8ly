package zitadeladmin

import "testing"

func TestNew_RejectsMissingConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"empty BaseURL", Config{Token: "t", OrgID: "o"}},
		{"empty Token", Config{BaseURL: "https://auth.example.com", OrgID: "o"}},
		{"empty OrgID", Config{BaseURL: "https://auth.example.com", Token: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.cfg, nil); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestNew_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	c, err := New(Config{BaseURL: "https://auth.example.com/", Token: "t", OrgID: "o"}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.baseURL != "https://auth.example.com" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}
