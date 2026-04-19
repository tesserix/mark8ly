// Package taxreg builds a populated *tax.Registry from a config. It exists in
// its own package to avoid an import cycle (the tax package owns the Validator
// interface and is imported by every validators/{country}.go; if NewRegistry
// lived in tax, tax → validators → tax would cycle).
package taxreg

import (
	"net/http"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

// Config wires shared HTTP infrastructure and per-registry secrets into the
// country validator map. The HTTPClient is shared across validators so the
// connection pool is reused.
type Config struct {
	HTTPClient *http.Client

	// NZEnabled gates the New Zealand validator. Defaults to false until
	// counsel sign-off (§20.3); when false, NZ requests return ErrValidatorDisabled.
	NZEnabled bool

	// Per-registry overrides — empty falls back to the validator's hard-coded
	// production URL constant. Tests inject httptest.Server URLs here.
	HMRCBaseURL string
	VIESBaseURL string
	ABNBaseURL  string
	GSTNBaseURL string
	ACRABaseURL string
	IRDBaseURL  string

	// Per-registry credentials — empty means anonymous calls (works for
	// HMRC/VIES/ABN/ACRA which permit anonymous lookups; GSTN requires a
	// token in production).
	GSTNAuthToken string
	ABNGUID       string
}

// BuildDefault constructs a registry pre-populated with every supported
// country. The IE+EU group shares one VIES validator with WithCountry-bound
// copies; the NZ key returns the disabled sentinel when cfg.NZEnabled is false.
func BuildDefault(cfg Config) *tax.Registry {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	r := tax.NewRegistry()

	r.Set("US", validators.NewUS())
	r.Set("CA", validators.NewCA())
	r.Set("GB", validators.NewUK(cfg.HTTPClient, cfg.HMRCBaseURL))

	vies := validators.NewEU(cfg.HTTPClient, cfg.VIESBaseURL)
	for _, c := range []string{"IE", "DE", "FR", "IT", "ES", "NL"} {
		r.Set(c, vies.WithCountry(c))
	}

	au := validators.NewAU(cfg.HTTPClient, cfg.ABNBaseURL)
	if cfg.ABNGUID != "" {
		au = au.WithGUID(cfg.ABNGUID)
	}
	r.Set("AU", au)

	if cfg.NZEnabled {
		r.Set("NZ", validators.NewNZ(cfg.HTTPClient, cfg.IRDBaseURL))
	} else {
		r.Set("NZ", validators.NewNZDisabled())
	}

	in := validators.NewIN(cfg.HTTPClient, cfg.GSTNBaseURL)
	if cfg.GSTNAuthToken != "" {
		in = in.WithAuthToken(cfg.GSTNAuthToken)
	}
	r.Set("IN", in)
	r.Set("SG", validators.NewSG(cfg.HTTPClient, cfg.ACRABaseURL))

	// SEA — manual review queue, no upstream API.
	r.Set("MY", validators.NewMY())
	r.Set("TH", validators.NewTH())
	r.Set("PH", validators.NewPH())
	r.Set("ID", validators.NewID())
	r.Set("VN", validators.NewVN())

	return r
}
