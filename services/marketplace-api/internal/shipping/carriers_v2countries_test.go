package shipping

import (
	"testing"
)

// P18 §4.1 / §4.1.1: ShipEngine must route IE + NZ; NinjaVan must route VN.
// AE is explicitly out of scope (Aramex deferred to v2 per §2).

func TestShipEngine_SupportedCountries_IncludesIEAndNZ(t *testing.T) {
	c := NewShipEngineCarrier("test-key", "test")
	got := c.SupportedCountries()
	for _, want := range []string{"IE", "NZ"} {
		if !containsStr(got, want) {
			t.Errorf("ShipEngine.SupportedCountries() missing %q; got %v", want, got)
		}
	}
	if containsStr(got, "AE") {
		t.Errorf("ShipEngine.SupportedCountries() unexpectedly contains AE; Aramex is deferred to v2")
	}
}

func TestNinjaVan_SupportedCountries_IncludesVN(t *testing.T) {
	c := NewNinjaVanCarrier("k", "s", "test", "VN")
	got := c.SupportedCountries()
	if !containsStr(got, "VN") {
		t.Errorf("NinjaVan.SupportedCountries() missing VN; got %v", got)
	}
}

func TestResolveForCountry_IE_RoutesToShipEngine(t *testing.T) {
	se := NewShipEngineCarrier("k", "test")
	nv := NewNinjaVanCarrier("k", "s", "test", "SG")
	matched := ResolveForCountry("IE", []Carrier{nv, se})
	if len(matched) != 1 || matched[0].ProviderName() != "shipengine" {
		t.Fatalf("ResolveForCountry(IE) = %v, want [shipengine]", providerNames(matched))
	}
}

func TestResolveForCountry_NZ_RoutesToShipEngine(t *testing.T) {
	se := NewShipEngineCarrier("k", "test")
	nv := NewNinjaVanCarrier("k", "s", "test", "SG")
	matched := ResolveForCountry("NZ", []Carrier{nv, se})
	if len(matched) != 1 || matched[0].ProviderName() != "shipengine" {
		t.Fatalf("ResolveForCountry(NZ) = %v, want [shipengine]", providerNames(matched))
	}
}

func TestResolveForCountry_VN_RoutesToNinjaVan(t *testing.T) {
	se := NewShipEngineCarrier("k", "test")
	nv := NewNinjaVanCarrier("k", "s", "test", "VN")
	matched := ResolveForCountry("VN", []Carrier{se, nv})
	if len(matched) != 1 || matched[0].ProviderName() != "ninjavan" {
		t.Fatalf("ResolveForCountry(VN) = %v, want [ninjavan]", providerNames(matched))
	}
}

// P18 Task 3 audit: NewCarrier("ninjavan", ...) without a countryCode must
// error rather than silently defaulting to "sg". Prevents cross-country
// silent routing bugs when e.g. a VN merchant's carrier config is mis-wired.
func TestNewCarrier_NinjaVan_RequiresCountryCode(t *testing.T) {
	_, err := NewCarrier("ninjavan", "k", "s", "test")
	if err == nil {
		t.Fatal("NewCarrier(ninjavan) without countryCode = nil err; want error")
	}
	_, err = NewCarrier("ninjavan", "k", "s", "test", "")
	if err == nil {
		t.Fatal("NewCarrier(ninjavan, \"\") = nil err; want error")
	}
	_, err = NewCarrier("ninjavan", "k", "s", "test", "VN")
	if err != nil {
		t.Fatalf("NewCarrier(ninjavan, \"VN\") = %v; want no error", err)
	}
}

func TestNewCarrier_ShipEngine_AcceptsIEAndNZ(t *testing.T) {
	for _, cc := range []string{"IE", "NZ"} {
		c, err := NewCarrier("shipengine", "k", "", "test")
		if err != nil {
			t.Fatalf("NewCarrier(shipengine) = %v", err)
		}
		matched := ResolveForCountry(cc, []Carrier{c})
		if len(matched) != 1 {
			t.Errorf("NewCarrier(shipengine) does not resolve for %q", cc)
		}
	}
}

func containsStr(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func providerNames(cs []Carrier) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ProviderName())
	}
	return out
}
