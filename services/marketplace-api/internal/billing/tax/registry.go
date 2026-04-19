package tax

// Registry maps ISO-3166 alpha-2 country codes to validators. Construction
// (which depends on the concrete validators package) lives in the sibling
// taxreg package to avoid an import cycle: validators depend on this package
// for the Validator interface and error sentinels.
type Registry struct {
	byCountry map[string]Validator
}

// NewRegistry builds an empty registry. Callers populate it via Set; the
// taxreg package provides BuildDefault, which wires every supported country
// validator from cfg.
func NewRegistry() *Registry {
	return &Registry{byCountry: map[string]Validator{}}
}

// Set binds a validator to a country code. Idempotent; the latest binding wins.
func (r *Registry) Set(country string, v Validator) {
	r.byCountry[country] = v
}

// For returns the validator for a country, or (nil, false) when unsupported.
func (r *Registry) For(country string) (Validator, bool) {
	v, ok := r.byCountry[country]
	return v, ok
}

// Countries returns the registered country codes (ordering not guaranteed).
// Test-only convenience for asserting completeness.
func (r *Registry) Countries() []string {
	out := make([]string, 0, len(r.byCountry))
	for c := range r.byCountry {
		out = append(out, c)
	}
	return out
}
