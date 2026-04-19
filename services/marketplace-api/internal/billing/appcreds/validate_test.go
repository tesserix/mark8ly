package appcreds

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
)

// generateP256P8 mints a fresh PKCS#8-encoded P-256 private key in PEM
// form — the exact shape Apple's .p8 files take. Used to produce valid
// fixtures without embedding pre-generated keys in the repo (and thus
// without triggering the "private key in source" scanners).
func generateP256P8(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa p256: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestValidateP8_AcceptsES256PrivateKey(t *testing.T) {
	if err := ValidateP8(generateP256P8(t)); err != nil {
		t.Errorf("ValidateP8(valid p256) = %v, want nil", err)
	}
}

func TestValidateP8_RejectsWrongCurve(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("gen p384: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	p8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	err = ValidateP8(p8)
	if err == nil {
		t.Fatal("ValidateP8(p384) = nil, want ErrInvalidP8")
	}
	if !errors.Is(err, ErrInvalidP8) {
		t.Errorf("err = %v, want wraps ErrInvalidP8", err)
	}
}

func TestValidateP8_RejectsRSAKey(t *testing.T) {
	// An "RSA PRIVATE KEY" PEM block — Apple would reject this at signing.
	err := ValidateP8([]byte(
		"-----BEGIN RSA PRIVATE KEY-----\nABCD\n-----END RSA PRIVATE KEY-----\n"))
	if err == nil {
		t.Fatal("ValidateP8(rsa-labelled) = nil, want ErrInvalidP8")
	}
	if !errors.Is(err, ErrInvalidP8) {
		t.Errorf("err = %v, want wraps ErrInvalidP8", err)
	}
}

func TestValidateP8_RejectsGarbage(t *testing.T) {
	err := ValidateP8([]byte("not a pem file"))
	if err == nil {
		t.Fatal("ValidateP8(garbage) = nil, want ErrInvalidP8")
	}
	if !errors.Is(err, ErrInvalidP8) {
		t.Errorf("err = %v, want wraps ErrInvalidP8", err)
	}
}

func TestValidateP8_RejectsEmpty(t *testing.T) {
	err := ValidateP8(nil)
	if err == nil {
		t.Fatal("ValidateP8(nil) = nil, want ErrInvalidP8")
	}
	err = ValidateP8([]byte(""))
	if err == nil {
		t.Fatal("ValidateP8(\"\") = nil, want ErrInvalidP8")
	}
}

func TestValidateGooglePlayJSON_AcceptsServiceAccount(t *testing.T) {
	valid := []byte(`{
	  "type":"service_account",
	  "project_id":"my-proj",
	  "private_key_id":"kid",
	  "private_key":"-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----",
	  "client_email":"sa@my-proj.iam.gserviceaccount.com",
	  "client_id":"1",
	  "token_uri":"https://oauth2.googleapis.com/token"
	}`)
	if err := ValidateGooglePlayJSON(valid); err != nil {
		t.Errorf("ValidateGooglePlayJSON(valid) = %v, want nil", err)
	}
}

func TestValidateGooglePlayJSON_RejectsUserCred(t *testing.T) {
	user := []byte(`{"type":"authorized_user","client_id":"x","client_secret":"y","refresh_token":"z"}`)
	err := ValidateGooglePlayJSON(user)
	if err == nil {
		t.Fatal("ValidateGooglePlayJSON(authorized_user) = nil, want ErrInvalidGooglePlayJSON")
	}
	if !errors.Is(err, ErrInvalidGooglePlayJSON) {
		t.Errorf("err = %v, want wraps ErrInvalidGooglePlayJSON", err)
	}
}

func TestValidateGooglePlayJSON_RejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"no project_id", `{"type":"service_account","private_key":"k","client_email":"e@x.iam"}`},
		{"no private_key", `{"type":"service_account","project_id":"p","client_email":"e@x.iam"}`},
		{"no client_email", `{"type":"service_account","project_id":"p","private_key":"k"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateGooglePlayJSON([]byte(tc.payload)); err == nil {
				t.Fatal("ValidateGooglePlayJSON(missing field) = nil, want error")
			}
		})
	}
}

func TestValidateGooglePlayJSON_RejectsInvalidJSON(t *testing.T) {
	err := ValidateGooglePlayJSON([]byte("not json"))
	if err == nil {
		t.Fatal("ValidateGooglePlayJSON(not json) = nil, want error")
	}
	if !errors.Is(err, ErrInvalidGooglePlayJSON) {
		t.Errorf("err = %v, want wraps ErrInvalidGooglePlayJSON", err)
	}
}
