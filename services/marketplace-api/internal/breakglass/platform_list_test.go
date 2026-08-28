package breakglass_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
)

// TestPlatformRowCannotCarryCredentialFields is the core guarantee behind
// the platform break-glass read (#333): break_glass_accounts holds
// secret_path (a GCP Secret Manager path pointing at the live plaintext
// password and TOTP secret), password_hash (bcrypt), and totp_secret_ref (a
// JSON pointer into that blob). None of the three may ever be reachable
// from breakglass.PlatformRow, by field name or by JSON tag, and a field
// merely containing "secret", "hash", or "password" is forbidden outright
// so a differently-named credential column added later still trips this
// guard.
func TestPlatformRowCannotCarryCredentialFields(t *testing.T) {
	exact := []string{"secret_path", "password_hash", "totp_secret_ref"}
	substrings := []string{"secret", "hash", "password"}

	tp := reflect.TypeOf(breakglass.PlatformRow{})
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		tag := f.Tag.Get("json")
		name := strings.SplitN(tag, ",", 2)[0]

		for _, bad := range exact {
			if name == bad {
				t.Errorf("PlatformRow.%s has forbidden json tag %q", f.Name, name)
			}
			if strings.EqualFold(f.Name, bad) {
				t.Errorf("PlatformRow.%s has forbidden field name %q", f.Name, f.Name)
			}
		}
		for _, sub := range substrings {
			if strings.Contains(strings.ToLower(name), sub) {
				t.Errorf("PlatformRow.%s json tag %q contains forbidden substring %q", f.Name, name, sub)
			}
			if strings.Contains(strings.ToLower(f.Name), sub) {
				t.Errorf("PlatformRow.%s field name contains forbidden substring %q", f.Name, sub)
			}
		}
	}
}
