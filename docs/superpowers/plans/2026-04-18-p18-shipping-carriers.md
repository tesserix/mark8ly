# P18 — Shipping Carriers: Add IE + NZ (ShipEngine) and VN (NinjaVan) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing `internal/shipping` integrations to cover the three new v2 countries called out in spec §2 and §25: **Ireland (IE) + New Zealand (NZ) via ShipEngine**, and **Vietnam (VN) via NinjaVan**. Defer UAE / Aramex to v2 per spec. Touch only configuration, country whitelists, address-validation quirks, carrier-service-code tables, and shipping-zone seed rows — no algorithm changes.

**Architecture:** Both ShipEngine and NinjaVan integrations already expose a country-keyed dispatch surface (`map[CountryCode]carrierConfig` or equivalent). The work is to (a) register three new country codes with the correct carrier-account IDs + default service codes pulled from config/env, (b) audit and fix the NinjaVan client's likely-hardcoded `SG` default for cross-country POST calls, (c) relax any address-validation rule that rejects NZ's missing state/province, and (d) seed one `shipping_zones` row per new country so the existing admin UI can assign carriers without code changes. Each integration is covered by fixture-based unit tests (mocked carrier HTTP responses in `testdata/`) plus an integration test that hits a real `testdb.NewDB(t, "shipping_zones")` row to verify zone → carrier resolution.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, existing `internal/shipping/{shipengine,delhivery,ninjavan}.go` clients, `pkg/config/config.go` for carrier credentials, `pkg/testdb` for integration fixtures. No new third-party dependencies — ShipEngine and NinjaVan SDKs (or thin REST wrappers) already in-tree handle IE/NZ/VN natively at the protocol level.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §2 last row ("Shipping carriers ... Add IE, NZ to ShipEngine; VN to NinjaVan; defer AE"), §4.1 country shipping column (IE row: `ShipEngine`, NZ row: `ShipEngine`), §4.1.1 emerging-markets row (VN: `NinjaVan`), §25 effort table lines 1048-1050 ("Add IE, NZ to ShipEngine — 1 day", "Add VN to NinjaVan — 1 day", "AE / Aramex (deferred v2) — 1 week").

**Depends on:** Nothing in P1–P17. This phase is orthogonal to the subscription stack. Only dependency is the existing carrier-client code on `main`.

**Related plans:** None. UAE / Aramex lands post-v1 as its own separate phase.

---

## Scope Check

In scope:
1. **ShipEngine IE:** register Ireland country code; confirm address validation accepts IE Eircode format; add default service codes (An Post domestic + international carriers fronted by ShipEngine); sample label creation via sandbox.
2. **ShipEngine NZ:** register New Zealand country code; **relax address validation so a missing state/province does not reject** (NZ addresses have no state field); accept `RD<n>` rural-delivery markers in address line 2; add default service codes (NZ Post domestic + international).
3. **NinjaVan VN:** register Vietnam; audit NinjaVan client for any hardcoded `SG` / `country_code` defaults on POST paths and convert them to per-call parameters; add VN-specific service codes; test-create a pickup request + label via sandbox.
4. **`shipping_zones` database seeding:** add one row per new country (IE, NZ, VN) mapping `country_code → carrier_id` so the existing admin zone-assignment UI renders the new countries. If `shipping_zones` table does not yet exist in the marketplace-api schema, add the table in a forward-only migration as part of this plan.
5. **`pkg/config/config.go`:** add config keys for ShipEngine IE carrier-account ID, ShipEngine NZ carrier-account ID, and NinjaVan VN sub-account credentials. Secrets themselves are already provisioned in GCP Secret Manager — this plan only wires the env-var reads.
6. **Explicit AE / Aramex deferral note** at the bottom of this plan citing spec §2 + §25 line 1050.

Out of scope:
- Aramex / UAE integration (deferred to v2 per spec §2 + §25).
- Rate-calculation algorithm changes. The existing `quoteRates` / weight-bracket logic applies to IE/NZ/VN without modification.
- Admin UI changes for configuring carriers per zone. The existing UI enumerates zones from the database — once the new rows exist, it will render them.
- Cross-border / international lane matrices (e.g. IE → US). v2 only uses each carrier for *domestic* shipping within the merchant's country.
- Refund / reverse-logistics flows. New countries inherit existing behaviour.

---

## File Structure

### Create

- `services/marketplace-api/internal/shipping/testdata/ie_label_create_success.json` — ShipEngine IE sandbox fixture
- `services/marketplace-api/internal/shipping/testdata/ie_address_validate_success.json`
- `services/marketplace-api/internal/shipping/testdata/nz_label_create_success.json`
- `services/marketplace-api/internal/shipping/testdata/nz_address_validate_no_state.json` — proves the NZ no-state case is accepted
- `services/marketplace-api/internal/shipping/testdata/nz_address_validate_rd_number.json` — rural-delivery marker case
- `services/marketplace-api/internal/shipping/testdata/vn_pickup_create_success.json`
- `services/marketplace-api/internal/shipping/testdata/vn_label_create_success.json`
- `services/marketplace-api/internal/shipping/shipengine_ie_nz_test.go` — fixture-driven unit tests
- `services/marketplace-api/internal/shipping/ninjavan_vn_test.go` — fixture-driven unit tests
- `services/marketplace-api/internal/shipping/zones_lookup_test.go` — `//go:build integration` test hitting `testdb.NewDB(t, "shipping_zones")`
- `services/marketplace-api/migrations/000047_shipping_zones_ie_nz_vn.up.sql` — seed rows (+ CREATE TABLE if absent per exploration)
- `services/marketplace-api/migrations/000047_shipping_zones_ie_nz_vn.down.sql` — DELETE the three rows (preserve table if it pre-exists)

### Modify

- `services/marketplace-api/internal/shipping/shipengine.go` — extend country whitelist + service-code table; relax NZ address validation
- `services/marketplace-api/internal/shipping/ninjavan.go` — add VN config block; fix hardcoded `SG` defaults on POST calls (audit required — see Task 3 Step 2)
- `services/marketplace-api/pkg/config/config.go` — add `ShipEngineCarrierAccountIE`, `ShipEngineCarrierAccountNZ`, `NinjaVanVNAPIKey`, `NinjaVanVNClientID`, `NinjaVanVNClientSecret` fields + env-var bindings

### Delete

- Nothing.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Config wiring: add IE/NZ/VN carrier credentials to `pkg/config` | — |
| 2 | ShipEngine IE + NZ: whitelist, service codes, NZ no-state address validation | 1 |
| 3 | NinjaVan VN: audit hardcoded `SG` defaults, add VN config + service codes | 1 |
| 4 | `shipping_zones` migration: add table-if-missing + seed IE/NZ/VN rows | — |
| 5 | Integration + manual-smoke: zone lookup returns correct carrier; sandbox labels print | 2, 3, 4 |

Five tasks total. Small, focused, ships in one or two sittings.

---

## Reusable patterns

**A. Country-keyed carrier config** — Both `shipengine.go` and `ninjavan.go` already key per-country configuration off an enum or `map[string]config`. Adding a country means one new map entry per integration; no branch-on-country code elsewhere.

**B. Fixture-based HTTP mocking** — Carrier HTTP calls go through an interface (`httpDoer` or `*http.Client`). Tests swap in a `roundTripperFunc` that reads `testdata/*.json` and returns it as the response body. No real sandbox calls in unit tests — only in the manual smoke step.

**C. `//go:build integration` + `testdb.NewDB`** — Integration tests tagged `integration` get a fresh database with the named tables migrated in. Seed rows via `db.Exec` inside the test. Pattern already in use across the repo.

**D. Forward-only migration with idempotent seed** — Use `INSERT ... ON CONFLICT (country_code) DO NOTHING` so re-running the migration in dev is safe. `CREATE TABLE IF NOT EXISTS` guards the zones-table-creation case.

**E. AE deferral annotation** — A single code comment (`// TODO(v2): Aramex AE — see spec §2, §25 effort table`) placed in `shipengine.go` and `ninjavan.go` near the country whitelist. No other AE references added.

---

## Task 1: Config wiring — ShipEngine IE/NZ carrier-account IDs + NinjaVan VN credentials

**Files:**
- Modify: `services/marketplace-api/pkg/config/config.go`
- Modify: `services/marketplace-api/pkg/config/config_test.go`

**Spec references:** §2 last row; §4.1 IE/NZ rows; §4.1.1 VN row.

**Objective:** Add five new config fields + their env-var bindings. The secrets themselves are already provisioned in GCP Secret Manager and mounted via ESO; this task only wires Go-side reads.

- [ ] **Step 1: Write failing test — new fields load from env**

```go
func TestConfig_LoadsShippingCarrierCredentials(t *testing.T) {
    t.Setenv("SHIPENGINE_CARRIER_ACCOUNT_IE", "se-ie-acct-123")
    t.Setenv("SHIPENGINE_CARRIER_ACCOUNT_NZ", "se-nz-acct-456")
    t.Setenv("NINJAVAN_VN_API_KEY", "nv-vn-key")
    t.Setenv("NINJAVAN_VN_CLIENT_ID", "nv-vn-cid")
    t.Setenv("NINJAVAN_VN_CLIENT_SECRET", "nv-vn-secret")

    cfg, err := config.Load()
    require.NoError(t, err)
    require.Equal(t, "se-ie-acct-123", cfg.ShipEngineCarrierAccountIE)
    require.Equal(t, "se-nz-acct-456", cfg.ShipEngineCarrierAccountNZ)
    require.Equal(t, "nv-vn-key", cfg.NinjaVanVNAPIKey)
    require.Equal(t, "nv-vn-cid", cfg.NinjaVanVNClientID)
    require.Equal(t, "nv-vn-secret", cfg.NinjaVanVNClientSecret)
}
```

- [ ] **Step 2: Run — expect FAIL (fields don't exist)**

```bash
cd services/marketplace-api
go test ./pkg/config/... -run TestConfig_LoadsShippingCarrierCredentials -v
```

- [ ] **Step 3: Add fields to `Config` struct**

Add to the shipping block (grouped with existing ShipEngine/NinjaVan fields):

```go
type Config struct {
    // ... existing ...
    ShipEngineCarrierAccountIE string `env:"SHIPENGINE_CARRIER_ACCOUNT_IE"`
    ShipEngineCarrierAccountNZ string `env:"SHIPENGINE_CARRIER_ACCOUNT_NZ"`
    NinjaVanVNAPIKey           string `env:"NINJAVAN_VN_API_KEY"`
    NinjaVanVNClientID         string `env:"NINJAVAN_VN_CLIENT_ID"`
    NinjaVanVNClientSecret     string `env:"NINJAVAN_VN_CLIENT_SECRET"`
}
```

Adjust the struct tags to match the existing tag style in `config.go` (could be `mapstructure:"..."` / `envconfig:"..."` — match what the file already does; pick one pattern, do not mix).

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/pkg/config/config{,_test}.go
git commit -m "feat(shipping): add config fields for ShipEngine IE/NZ + NinjaVan VN credentials"
```

---

## Task 2: ShipEngine — whitelist IE + NZ, relax NZ address validation

**Files:**
- Modify: `services/marketplace-api/internal/shipping/shipengine.go`
- Create: `services/marketplace-api/internal/shipping/shipengine_ie_nz_test.go`
- Create: `services/marketplace-api/internal/shipping/testdata/ie_label_create_success.json`
- Create: `services/marketplace-api/internal/shipping/testdata/ie_address_validate_success.json`
- Create: `services/marketplace-api/internal/shipping/testdata/nz_label_create_success.json`
- Create: `services/marketplace-api/internal/shipping/testdata/nz_address_validate_no_state.json`
- Create: `services/marketplace-api/internal/shipping/testdata/nz_address_validate_rd_number.json`

**Spec references:** §4.1 Ireland row; §4.1 New Zealand row.

**Key implementation details:**
- ShipEngine's REST API handles `IE` and `NZ` country codes natively. Our integration's country whitelist is a pre-dispatch guard in `shipengine.go` — it must allow `IE` and `NZ` through.
- NZ addresses **have no state/province field at all**. Any validation rule that requires `state != ""` for ShipEngine-routed countries will reject every NZ address. Audit `ValidateAddress` / `normalizeAddress` for such checks and exempt NZ.
- NZ rural-delivery addresses use an `RD<n>` marker (e.g., `RD 2`) in address line 2. ShipEngine accepts this natively; our validator must not strip or reject it.

- [ ] **Step 1: Fixture setup — write the five JSON files**

For each, use a minimal but realistic ShipEngine response body:

`testdata/ie_label_create_success.json`:
```json
{
  "label_id": "se-lbl-ie-001",
  "status": "completed",
  "carrier_code": "an_post",
  "service_code": "an_post_parcel",
  "tracking_number": "RA123456789IE",
  "ship_to": { "country_code": "IE", "postal_code": "D02 XY45" }
}
```

`testdata/nz_label_create_success.json` — same shape, `"country_code": "NZ"`, `carrier_code: "nz_post"`, `service_code: "nz_post_tracked"`, postal code `"6011"`, **no `state_province` field**.

`testdata/nz_address_validate_no_state.json` — ShipEngine validate response where request omitted `state_province`, response sets `"status": "verified"`, `"messages": []`.

`testdata/nz_address_validate_rd_number.json` — address line 2 = `"RD 2"`, response `"status": "verified"`.

`testdata/ie_address_validate_success.json` — Dublin 2 postal code D02 XY45, `"status": "verified"`.

- [ ] **Step 2: Failing test — IE label creation round-trips through the client**

```go
func TestShipEngine_CreateLabel_IE(t *testing.T) {
    rt := fixtureRoundTripper(t, "testdata/ie_label_create_success.json", 200)
    cli := shipping.NewShipEngineClient(&http.Client{Transport: rt}, shipping.ShipEngineConfig{
        APIKey:               "test",
        CarrierAccountByCountry: map[string]string{"IE": "se-ie-acct-123"},
    })

    lbl, err := cli.CreateLabel(context.Background(), shipping.LabelRequest{
        CountryCode: "IE",
        ToAddress:   shipping.Address{CountryCode: "IE", PostalCode: "D02 XY45", City: "Dublin"},
        // ... minimal valid package ...
    })
    require.NoError(t, err)
    require.Equal(t, "se-lbl-ie-001", lbl.ID)
    require.Equal(t, "RA123456789IE", lbl.TrackingNumber)
}
```

- [ ] **Step 3: Failing test — NZ address without state is accepted**

```go
func TestShipEngine_ValidateAddress_NZ_NoStateAccepted(t *testing.T) {
    rt := fixtureRoundTripper(t, "testdata/nz_address_validate_no_state.json", 200)
    cli := shipping.NewShipEngineClient(&http.Client{Transport: rt}, shipping.ShipEngineConfig{
        APIKey: "test", CarrierAccountByCountry: map[string]string{"NZ": "se-nz-acct-456"},
    })

    res, err := cli.ValidateAddress(context.Background(), shipping.Address{
        CountryCode: "NZ", PostalCode: "6011", City: "Wellington",
        // StateProvince deliberately empty.
    })
    require.NoError(t, err)
    require.True(t, res.Verified)
}
```

- [ ] **Step 4: Failing test — NZ RD rural-delivery marker is preserved**

```go
func TestShipEngine_ValidateAddress_NZ_RDNumberPreserved(t *testing.T) {
    rt := fixtureRoundTripper(t, "testdata/nz_address_validate_rd_number.json", 200)
    cli := shipping.NewShipEngineClient(&http.Client{Transport: rt}, shipping.ShipEngineConfig{
        APIKey: "test", CarrierAccountByCountry: map[string]string{"NZ": "se-nz-acct-456"},
    })

    res, err := cli.ValidateAddress(context.Background(), shipping.Address{
        CountryCode: "NZ", PostalCode: "3197", City: "Thames",
        AddressLine2: "RD 2",
    })
    require.NoError(t, err)
    require.True(t, res.Verified)
}
```

- [ ] **Step 5: Run — expect FAIL**

```bash
go test ./internal/shipping/... -run 'ShipEngine_.*(IE|NZ)' -v
```

- [ ] **Step 6: Extend `shipengine.go`**

Three changes:

1. **Country whitelist:** add `"IE"` and `"NZ"` to the allowed-country set. Keep entries alphabetical.
2. **Service-code map:** add default domestic service codes for IE (`an_post_parcel`) and NZ (`nz_post_tracked`). These are the fallbacks used when the merchant has not explicitly picked one in admin UI.
3. **Address validation pre-check:** locate the pre-send validator (likely `validateOutboundAddress(a Address) error`). The existing check that rejects empty `StateProvince` for non-US countries must exempt `NZ`. Change:

```go
// Before (illustrative):
if a.CountryCode != "US" && a.StateProvince == "" {
    return fmt.Errorf("state_province required for country %s", a.CountryCode)
}

// After:
if requiresStateProvince(a.CountryCode) && a.StateProvince == "" {
    return fmt.Errorf("state_province required for country %s", a.CountryCode)
}

// where:
func requiresStateProvince(cc string) bool {
    switch cc {
    case "NZ", "IE", "GB": // single-tier-admin or postcode-routed countries
        return false
    default:
        return true
    }
}
```

GB already works (existing country); confirm by running existing GB tests. IE added as a no-state country along with NZ — Ireland's administrative divisions (counties) are optional in postal addresses and not required by ShipEngine.

Add the AE deferral marker:

```go
// TODO(v2): Aramex AE integration — deferred per spec §2 last row and §25 effort
// table ("AE / Aramex (deferred v2) — 1 week"). Do not add AE to this whitelist.
```

- [ ] **Step 7: Run — expect PASS (all three new tests + existing tests)**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/shipping/shipengine.go \
        services/marketplace-api/internal/shipping/shipengine_ie_nz_test.go \
        services/marketplace-api/internal/shipping/testdata/ie_*.json \
        services/marketplace-api/internal/shipping/testdata/nz_*.json
git commit -m "feat(shipping): add ShipEngine IE + NZ support with NZ no-state address handling"
```

---

## Task 3: NinjaVan — audit hardcoded `SG` defaults, add VN config + service codes

**Files:**
- Modify: `services/marketplace-api/internal/shipping/ninjavan.go`
- Create: `services/marketplace-api/internal/shipping/ninjavan_vn_test.go`
- Create: `services/marketplace-api/internal/shipping/testdata/vn_pickup_create_success.json`
- Create: `services/marketplace-api/internal/shipping/testdata/vn_label_create_success.json`

**Spec references:** §4.1.1 Vietnam row (NinjaVan).

**Key implementation details:**
- NinjaVan's API is country-partitioned: the base URL and the `country_code` parameter on POST calls vary per country (SG, MY, TH, PH, ID, VN). Service codes differ per country (e.g. SG's `NV_STANDARD` vs VN's equivalent). Verify VN service codes against NinjaVan's public API docs in the `testdata` fixtures.
- **Audit step (Step 2 below) is the real work here**, not the new country rows. If the existing client hardcodes `country_code: "SG"` on POST bodies or uses the SG base URL as default, VN calls will silently route to SG and fail auth. Convert hardcoded defaults to per-call parameters driven from the country-keyed config map.

- [ ] **Step 1: Write fixtures**

`testdata/vn_pickup_create_success.json`:
```json
{
  "requested_tracking_number": "NV-VN-001-2026",
  "reservation_id": "res-vn-001",
  "status": "confirmed",
  "country_code": "VN"
}
```

`testdata/vn_label_create_success.json`:
```json
{
  "tracking_number": "NV-VN-001-2026",
  "label_url": "https://sandbox.ninjavan.co/labels/vn/NV-VN-001-2026.pdf",
  "service_type": "Standard",
  "country_code": "VN"
}
```

- [ ] **Step 2: Audit existing `ninjavan.go` for hardcoded country assumptions**

Grep (mental model — do not run in plan):
- Search for `"SG"` literals, `"sg"`, `countryCode = `, `country_code = "sg"`, and base-URL constants like `ninjavan.co/SG/`.
- Any occurrence inside a function body (not inside a per-country config map) is a bug for VN. Convert to per-call parameter reading `cfg.CountryCode`.
- Document findings inline in Step 4's commit message. If the audit finds **zero** hardcoded defaults, note "audit clean" in the commit and proceed.

Expected finding (hypothesis — verify against code):
```go
// Likely current shape:
func (c *Client) CreatePickup(ctx context.Context, req PickupRequest) (*PickupResponse, error) {
    body := map[string]any{
        "country_code": "SG", // <-- hardcoded bug for VN
        "pickup_address": req.Address, // ...
    }
    // ...
}
```

Fix:
```go
func (c *Client) CreatePickup(ctx context.Context, req PickupRequest) (*PickupResponse, error) {
    cfg, ok := c.countryConfig[req.CountryCode]
    if !ok {
        return nil, fmt.Errorf("ninjavan: no config for country %q", req.CountryCode)
    }
    body := map[string]any{
        "country_code":   req.CountryCode,
        "pickup_address": req.Address,
    }
    url := cfg.BaseURL + "/pickups"
    // ... POST with cfg.APIKey / cfg.ClientID / cfg.ClientSecret ...
}
```

- [ ] **Step 3: Failing test — VN pickup + label creation**

```go
func TestNinjaVan_CreatePickup_VN(t *testing.T) {
    rt := fixtureRoundTripper(t, "testdata/vn_pickup_create_success.json", 200)
    cli := shipping.NewNinjaVanClient(&http.Client{Transport: rt}, shipping.NinjaVanConfig{
        CountryConfig: map[string]shipping.NinjaVanCountryConfig{
            "VN": {
                BaseURL:      "https://api-sandbox.ninjavan.co/VN",
                APIKey:       "nv-vn-key",
                ClientID:     "nv-vn-cid",
                ClientSecret: "nv-vn-secret",
                DefaultService: "Standard",
            },
        },
    })

    res, err := cli.CreatePickup(context.Background(), shipping.PickupRequest{
        CountryCode: "VN",
        Address:     shipping.Address{CountryCode: "VN", City: "Ho Chi Minh City", PostalCode: "700000"},
    })
    require.NoError(t, err)
    require.Equal(t, "NV-VN-001-2026", res.TrackingNumber)
    require.Equal(t, "VN", res.CountryCode)
}

func TestNinjaVan_CreateLabel_VN(t *testing.T) {
    // same shape; asserts label_url, service_type, country_code == "VN"
}

// Regression: SG still works after audit fixes.
func TestNinjaVan_CreatePickup_SG_StillWorks(t *testing.T) {
    // fixture for SG; ensures audit fixes didn't break the existing integration
}
```

- [ ] **Step 4: Run — expect FAIL**

```bash
go test ./internal/shipping/... -run 'NinjaVan_.*(VN|SG)' -v
```

- [ ] **Step 5: Implement fixes from Step 2 audit + add VN config block**

1. Convert `CreatePickup`, `CreateLabel`, `CancelShipment`, `TrackShipment` to read country from request; look up base URL + credentials from `countryConfig[req.CountryCode]`.
2. Add VN to the country whitelist alongside existing SG/MY/TH/PH/ID.
3. Wire VN credentials in the `NewNinjaVanClient` constructor — client pulls from `cfg.NinjaVanVNAPIKey` / `...ClientID` / `...ClientSecret`.
4. Add AE deferral comment:

```go
// TODO(v2): Aramex AE is NOT a NinjaVan country. NinjaVan serves SEA only.
// UAE coverage is deferred to v2 via the Aramex integration — see spec §2.
```

- [ ] **Step 6: Run — expect PASS (VN tests + SG regression test + any other existing country tests)**

- [ ] **Step 7: Commit**

Include audit findings in the commit body (still single-line per user preference):

```bash
git add services/marketplace-api/internal/shipping/ninjavan.go \
        services/marketplace-api/internal/shipping/ninjavan_vn_test.go \
        services/marketplace-api/internal/shipping/testdata/vn_*.json
git commit -m "feat(shipping): add NinjaVan VN support + fix hardcoded SG country defaults on POST calls"
```

---

## Task 4: `shipping_zones` migration — add-table-if-missing + seed IE/NZ/VN rows

**Files:**
- Create: `services/marketplace-api/migrations/000047_shipping_zones_ie_nz_vn.up.sql`
- Create: `services/marketplace-api/migrations/000047_shipping_zones_ie_nz_vn.down.sql`

**Spec references:** §4.1 IE/NZ, §4.1.1 VN.

**Key implementation details:**
- **Exploration first:** before writing this migration, search the existing migrations directory (`services/marketplace-api/migrations/*.sql`) for `shipping_zones`. Two cases:
  - **Case A — table exists:** write the migration as seed-only (`INSERT ... ON CONFLICT DO NOTHING`).
  - **Case B — table does not exist:** migration creates the table with columns `country_code TEXT PRIMARY KEY`, `carrier_id TEXT NOT NULL`, `default_service_code TEXT`, `currency TEXT`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, plus the three seed rows.
- Use migration number `000047` only if the next free slot is 47 — otherwise bump to match the actual next number on `main`. Rename the filename accordingly; the *content* of this plan is stable.
- Migration is forward-only (no `DROP TABLE` in `down.sql` if the table pre-existed); `down.sql` only removes the three seed rows to keep rollback safe.

- [ ] **Step 1: Determine the next migration number**

```bash
ls services/marketplace-api/migrations/ | sort | tail -5
```

Take the highest number, add 1. Replace `000047` everywhere in this task with the resolved number.

- [ ] **Step 2: Determine case A vs case B**

```bash
grep -l "shipping_zones" services/marketplace-api/migrations/*.sql || echo "table not yet defined"
```

- [ ] **Step 3: Write `up.sql` — Case B template (covers both cases)**

```sql
-- 000047_shipping_zones_ie_nz_vn.up.sql
-- Adds IE, NZ, VN shipping-zone rows per spec §4.1, §4.1.1.
-- Creates the shipping_zones table if it does not yet exist.

CREATE TABLE IF NOT EXISTS shipping_zones (
    country_code         TEXT PRIMARY KEY,
    carrier_id           TEXT NOT NULL,
    default_service_code TEXT,
    currency             TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO shipping_zones (country_code, carrier_id, default_service_code, currency)
VALUES
    ('IE', 'shipengine', 'an_post_parcel',    'EUR'),
    ('NZ', 'shipengine', 'nz_post_tracked',   'NZD'),
    ('VN', 'ninjavan',   'ninjavan_standard', 'VND')
ON CONFLICT (country_code) DO NOTHING;
```

If Case A (table exists already), strip the `CREATE TABLE` block; keep only the `INSERT`. If the existing table has different column names (e.g. `carrier_code` instead of `carrier_id`), adapt the `INSERT` to match — **do not alter the existing schema as part of this plan**.

- [ ] **Step 4: Write `down.sql`**

```sql
-- 000047_shipping_zones_ie_nz_vn.down.sql
-- Removes the three v2-shipping-rollout rows. Preserves the table regardless of
-- whether we created it in this migration (safer default — admin UI may already
-- have merchant-edited rows at rollback time).

DELETE FROM shipping_zones WHERE country_code IN ('IE', 'NZ', 'VN');
```

- [ ] **Step 5: Local smoke — apply + rollback + re-apply**

```bash
make migrate-up   # or the project's equivalent
psql "$DATABASE_URL" -c "SELECT country_code, carrier_id FROM shipping_zones WHERE country_code IN ('IE','NZ','VN');"
make migrate-down # should remove the three rows
make migrate-up   # idempotent; ON CONFLICT DO NOTHING handles repeat
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/migrations/000047_shipping_zones_ie_nz_vn.{up,down}.sql
git commit -m "feat(shipping): seed shipping_zones rows for IE, NZ, VN per v2 spec"
```

---

## Task 5: Integration test + manual sandbox smoke

**Files:**
- Create: `services/marketplace-api/internal/shipping/zones_lookup_test.go`

**Spec references:** §2, §4.1, §4.1.1.

**Objective:** Prove end-to-end that (a) a `shipping_zones` lookup for IE, NZ, VN returns the expected carrier, and (b) each new carrier integration completes a sandbox label / pickup call against the real carrier API.

- [ ] **Step 1: Integration test — zone lookup**

```go
//go:build integration

package shipping_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/shipping"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestShippingZoneLookup_NewCountries(t *testing.T) {
    db := testdb.NewDB(t, "shipping_zones")
    repo := shipping.NewZoneRepository(db)

    cases := []struct {
        country, wantCarrier, wantService, wantCurrency string
    }{
        {"IE", "shipengine", "an_post_parcel",    "EUR"},
        {"NZ", "shipengine", "nz_post_tracked",   "NZD"},
        {"VN", "ninjavan",   "ninjavan_standard", "VND"},
    }
    for _, tc := range cases {
        t.Run(tc.country, func(t *testing.T) {
            z, err := repo.GetByCountry(context.Background(), tc.country)
            require.NoError(t, err)
            require.Equal(t, tc.wantCarrier, z.CarrierID)
            require.Equal(t, tc.wantService, z.DefaultServiceCode)
            require.Equal(t, tc.wantCurrency, z.Currency)
        })
    }
}
```

If `ZoneRepository` / `GetByCountry` does not yet exist, that is itself a small addition — add a thin repo with only `GetByCountry(ctx, countryCode string) (Zone, error)`. Do not expand scope to full CRUD; admin UI already writes via its own path.

- [ ] **Step 2: Run integration test — expect PASS**

```bash
cd services/marketplace-api
go test -tags integration ./internal/shipping/... -run TestShippingZoneLookup_NewCountries -v
```

- [ ] **Step 3: Manual sandbox smoke — ShipEngine IE**

```bash
# With real sandbox credentials loaded into local env:
go run ./cmd/shipping-smoke -carrier=shipengine -country=IE \
    -to='{"country":"IE","postal":"D02 XY45","city":"Dublin","line1":"55 Grafton St"}' \
    -weight=500g
# Expect: label_id printed + PDF URL fetchable
```

If a `cmd/shipping-smoke` tool does not exist, skip this sub-step and rely on a one-off `_test.go` gated behind `//go:build sandbox` that performs the same call. Do not add the sandbox tool as new scope — it is a stretch step.

- [ ] **Step 4: Manual sandbox smoke — ShipEngine NZ (no-state address)**

Same as Step 3 with:
```
-to='{"country":"NZ","postal":"6011","city":"Wellington","line1":"100 Lambton Quay"}'
```
Critical: verify the sandbox accepts the address **with no `state` / `state_province` field**. If it rejects, revisit Task 2 Step 6 — the validator or the outbound request body is still sending an empty-string state field rather than omitting it.

- [ ] **Step 5: Manual sandbox smoke — NinjaVan VN**

```bash
go run ./cmd/shipping-smoke -carrier=ninjavan -country=VN \
    -pickup='{"country":"VN","city":"Ho Chi Minh City","postal":"700000","line1":"36 Nguyen Hue"}' \
    -weight=1kg
# Expect: tracking_number printed matching NV-VN-... format + label PDF URL
```

- [ ] **Step 6: Document sandbox findings**

Append a short block to the PR description (not a plan artefact) listing:
- IE: sandbox label-id created + tracking number prefix
- NZ: confirmed no-state body accepted; confirmed RD rural-delivery marker preserved in returned address
- VN: pickup reservation + label created; confirmed `country_code: "VN"` present on response (not silently rewritten to SG)

- [ ] **Step 7: Final commit**

```bash
git add services/marketplace-api/internal/shipping/zones_lookup_test.go
git commit -m "test(shipping): integration test for IE/NZ/VN shipping-zone lookup"
```

---

## Deferred: UAE / Aramex (v2)

Spec §2 (last row) and §25 effort table (line 1050) explicitly defer UAE to post-v1:

> AE / Aramex (deferred v2) — 1 week

Rationale:
- Aramex is not served by any existing integration (ShipEngine coverage of AE is partial and does not include the domestic carriers merchants actually use).
- A dedicated Aramex SDK/REST wrapper (~1 week of effort per spec) is out of scope for v1.
- Two deferral-marker comments are placed in `shipengine.go` and `ninjavan.go` (see Tasks 2 and 3, final steps).

Do not add AE to any country whitelist, service-code table, or `shipping_zones` seed in this plan. AE will land as its own P-phase post-v1.

---

## Rollout notes

- **Order of merge:** Tasks 1-3 are safe to merge independently of Task 4 (migration) — carrier clients compile and pass unit tests with no DB state. Task 5 requires Task 4 to have run on the target environment.
- **Feature flag:** not required. The new countries are inert until a merchant with a store in IE, NZ, or VN selects a shipping zone in admin UI. Existing merchants in other countries are unaffected.
- **Rollback:** revert Tasks 1-3 code changes; run `000047_shipping_zones_ie_nz_vn.down.sql` to remove seed rows. The table (if created by this migration) is deliberately **not** dropped on rollback — admin UI may have merchant-edited rows.
- **Manual verification after deploy:** pull a test merchant with a VN store, check the admin shipping-zones page renders Vietnam, and confirm a test order to a VN address successfully prints a NinjaVan label. Repeat for IE and NZ.
