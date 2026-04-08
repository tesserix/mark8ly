// Package mode owns the MODE env var parsing and validation.
// MODE selects which Gin engine(s) the marketplace-api binary constructs
// on startup: admin, storefront, or both. Two Knative Services deploy
// the same image with MODE=admin and MODE=storefront respectively; local
// dev uses MODE=both on a single port for convenience.
package mode

import "fmt"

// Mode is the deployment mode.
type Mode string

const (
	Admin      Mode = "admin"
	Storefront Mode = "storefront"
	Both       Mode = "both"
)

// Parse returns a Mode from a raw string, defaulting to Both when empty.
// Returns an error on any other unknown value.
func Parse(raw string) (Mode, error) {
	switch raw {
	case "":
		return Both, nil
	case string(Admin), string(Storefront), string(Both):
		return Mode(raw), nil
	default:
		return "", fmt.Errorf("mode: unknown MODE value %q (want admin|storefront|both)", raw)
	}
}

// RunsAdmin reports whether this mode serves admin routes.
func (m Mode) RunsAdmin() bool { return m == Admin || m == Both }

// RunsStorefront reports whether this mode serves storefront routes.
func (m Mode) RunsStorefront() bool { return m == Storefront || m == Both }
