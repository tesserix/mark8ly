# OpenBao Store (Phase 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make OpenBao the write target for per-tenant carrier credentials, while every existing `gsm://` row keeps resolving through GCP Secret Manager unchanged.

**Architecture:** A new `ChainStore` routes `Get` purely on the reference prefix (`bao://` → OpenBao, `gsm://` → GCP, `noop:`/`aes:` → the encryptor) and sends every `Put` to one configured primary. Because references are self-describing, dual-read is a property of the data model rather than a mode flag. A cache decorator wraps the whole thing. The existing `Store` interface is unchanged, so all 15 call sites are untouched.

**Tech Stack:** Go 1.26, `github.com/openbao/openbao/api/v2`, OpenBao KV v2, Kubernetes auth.

**Spec:** `docs/superpowers/specs/2026-09-03-openbao-carrier-secrets-design.md`

## Global Constraints

- OpenBao path: `kv/data/mark8ly/marketplace-api/tenants/<tenantID>/<domain>/<provider>/<field>`. No `<env>` segment.
- Reference format: `bao://kv/mark8ly/marketplace-api/tenants/<tenantID>/<domain>/<provider>/<field>` — **no version component**; rotation must return the identical reference.
- Cache TTL is **60 seconds**, stale-on-error. Both consequences are accepted and documented: plaintext lives in memory up to 60s, and a rotation takes up to 60s to take effect.
- **Writes never fall back.** If OpenBao is unavailable, `Put` fails. A silent fallback would mint `gsm://` references after cutover and make the phase-5 counter a lie.
- **A `bao://` reference never falls back to GCP.** Serve stale from cache if present, else fail closed.
- **Reads never gate readiness.** No OpenBao health check in `/ready`.
- The `Store` interface (`internal/carriersecrets/store.go:48`) must NOT change. No call site outside this package may be edited except `cmd/marketplace-api/main.go` wiring.
- Metrics use the repo's injected `CounterFn func(label string, increment int64)` pattern (see `internal/webhookprune/prune_cron.go:111`), never a package-level Prometheus global.
- Integration tests gate on an env var and must **skip loudly**, never pass silently.
- Every new test is verified to FAIL before it passes.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `internal/carriersecrets/refs.go` (modify) | Add `bao://` prefix, path builder, parser. Pure string logic, no I/O. |
| `internal/bao/client.go` (create) | OpenBao client: Kubernetes login, token cache + renew, error translation. Ported from secret-service. |
| `internal/carriersecrets/bao.go` (create) | `BaoClient` implementing the existing `SecretClient` interface over KV v2. |
| `internal/carriersecrets/chain.go` (create) | `ChainStore`: prefix routing, `Put` to primary, `Rewrapper`. |
| `internal/carriersecrets/cache.go` (create) | TTL cache decorator with stale-on-error. |
| `internal/carriersecrets/fakebao.go` (create) | In-memory fake so handler tests need no network. |
| `pkg/config/config.go` (modify) | OpenBao address, role, mount; adds `bao` to the existing `SHIPPING_SECRET_STORE`. |
| `cmd/marketplace-api/main.go` (modify) | Construct and wire the ChainStore. |

---

### Task 1: `bao://` references

Pure string logic with no I/O — the foundation every later task builds on.

**Files:**
- Modify: `internal/carriersecrets/refs.go`
- Test: `internal/carriersecrets/refs_test.go`

**Interfaces:**
- Consumes: `Scope` (`store.go:14`).
- Produces: `BaoRefPrefix`, `IsBaoRef(r string) bool`, `BaoPath(s Scope) string`, `FormatBaoReference(s Scope) string`, `ParseBaoReference(ref string) (path string, ok bool)`. Tasks 3, 4 and 5 use these names verbatim.

- [ ] **Step 1: Write the failing tests**

Add to `internal/carriersecrets/refs_test.go`:

```go
func TestBaoPath_IsScopeInPathForm(t *testing.T) {
	s := Scope{TenantID: "11111111-2222-3333-4444-555555555555", Domain: "payment", Provider: "razorpay", Field: "secret_key"}
	want := "kv/mark8ly/marketplace-api/tenants/11111111-2222-3333-4444-555555555555/payment/razorpay/secret_key"
	if got := BaoPath(s); got != want {
		t.Fatalf("BaoPath = %q, want %q", got, want)
	}
}

func TestFormatBaoReference_RoundTrips(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "shipping", Provider: "delhivery", Field: "api_key"}
	ref := FormatBaoReference(s)
	if !IsBaoRef(ref) {
		t.Fatalf("IsBaoRef(%q) = false", ref)
	}
	path, ok := ParseBaoReference(ref)
	if !ok || path != BaoPath(s) {
		t.Fatalf("ParseBaoReference(%q) = (%q, %v), want (%q, true)", ref, path, ok, BaoPath(s))
	}
}

// A bao reference must carry NO version: rotation writes a new KV v2 version
// at the same path and must return the identical reference, or every rotation
// becomes a DB-wide reference rewrite.
func TestFormatBaoReference_HasNoVersion(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "payment", Provider: "stripe", Field: "secret_key"}
	ref := FormatBaoReference(s)
	if strings.Contains(ref, "versions") {
		t.Fatalf("reference must not encode a version: %q", ref)
	}
	if FormatBaoReference(s) != ref {
		t.Fatal("FormatBaoReference must be deterministic for the same scope")
	}
}

// The prefixes must be mutually exclusive — ChainStore routing depends on it.
func TestRefPrefixes_AreMutuallyExclusive(t *testing.T) {
	s := Scope{TenantID: "t1", Domain: "payment", Provider: "razorpay", Field: "api_key"}
	bao := FormatBaoReference(s)
	if IsGSMRef(bao) || IsInlineRef(bao) {
		t.Fatalf("bao reference %q also matched another prefix", bao)
	}
	gsm := GSMRefPrefix + "projects/p/secrets/x"
	if IsBaoRef(gsm) {
		t.Fatalf("gsm reference %q matched IsBaoRef", gsm)
	}
	for _, inline := range []string{NoopRefPrefix + "abc", AESRefPrefix + "abc"} {
		if IsBaoRef(inline) {
			t.Fatalf("inline reference %q matched IsBaoRef", inline)
		}
	}
}

// Scope segments must be sanitised the same way GCP's are, so a stray slash
// cannot escape the tenant's subtree and reach another tenant's secret.
func TestBaoPath_SanitisesSegments(t *testing.T) {
	s := Scope{TenantID: "../other-tenant", Domain: "payment", Provider: "raz/orpay", Field: "api_key"}
	got := BaoPath(s)
	rest := strings.TrimPrefix(got, "kv/mark8ly/marketplace-api/tenants/")
	if strings.Contains(rest, "..") {
		t.Fatalf("path traversal survived sanitisation: %q", got)
	}
	if strings.Count(got, "/") != 7 {
		t.Fatalf("unexpected segment count in %q — a segment contained an unsanitised separator", got)
	}
}
```

Add `"strings"` to the test file's imports if absent.

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/carriersecrets/ -run 'TestBao|TestFormatBao|TestRefPrefixes'`
Expected: FAIL — `undefined: BaoPath`, `undefined: FormatBaoReference`.

- [ ] **Step 3: Implement**

Add to `internal/carriersecrets/refs.go`:

```go
// BaoRefPrefix marks an OpenBao KV v2 reference. The path that follows is
// the logical KV path WITHOUT the `data/` or `metadata/` infix — those are
// KV v2 API details the client adds, not part of the stored reference.
//
// Deliberately carries NO version: KV v2 writes a new version at the same
// path, so a rotation returns this identical reference. Encoding a version
// here would turn every rotation into a reference rewrite across the DB.
const BaoRefPrefix = "bao://"

// baoPathPrefix is the namespace-scoped root for every carrier credential.
// The estate convention is that the first path segment is the Kubernetes
// namespace (compare kv/data/homechef/*, kv/data/devai/devai-api/*), and
// environments are separated by cluster — so there is no env segment.
const baoPathPrefix = "kv/mark8ly/marketplace-api/tenants"

// IsBaoRef reports whether r is an OpenBao reference.
func IsBaoRef(r string) bool { return strings.HasPrefix(r, BaoRefPrefix) }

// BaoPath returns the logical KV path for a scope. Segments are sanitised
// with the same rules as the GCP secret ID so a stray '/' or '..' in a
// scope component cannot escape the tenant's subtree.
func BaoPath(s Scope) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		baoPathPrefix,
		sanitizeSegment(s.TenantID),
		sanitizeSegment(strings.ToLower(s.Domain)),
		sanitizeSegment(strings.ToLower(s.Provider)),
		sanitizeSegment(strings.ToLower(s.Field)),
	)
}

// FormatBaoReference returns the canonical reference persisted to the DB.
func FormatBaoReference(s Scope) string { return BaoRefPrefix + BaoPath(s) }

// ParseBaoReference extracts the logical KV path from a bao:// reference.
func ParseBaoReference(ref string) (path string, ok bool) {
	if !IsBaoRef(ref) {
		return "", false
	}
	return strings.TrimPrefix(ref, BaoRefPrefix), true
}
```

Also extend the package doc comment at the top of `refs.go` to list the new form alongside `gsm://` and `noop:/aes:`.

- [ ] **Step 4: Run the tests and confirm they pass**

Run: `go test ./internal/carriersecrets/ -run 'TestBao|TestFormatBao|TestRefPrefixes' -v`
Expected: all PASS.

- [ ] **Step 5: Run the whole package to confirm nothing regressed**

Run: `go test ./internal/carriersecrets/`
Expected: `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/carriersecrets/refs.go internal/carriersecrets/refs_test.go
git commit -m "feat(carriersecrets): add bao:// reference format and path builder"
```

---

### Task 2: Port the OpenBao client

**Files:**
- Create: `internal/bao/client.go`
- Test: `internal/bao/client_test.go`
- Modify: `go.mod` / `go.sum`

**Interfaces:**
- Consumes: nothing in this repo.
- Produces: `bao.Config{Address, KubernetesMount, KubernetesRole, ServiceAccountToken, Token string}`, `bao.New(cfg Config) (*Client, error)`, an accessor Task 3 uses to issue KV reads/writes, `(*Client).Health(ctx) error`, and sentinel errors `bao.ErrNotFound`, `bao.ErrForbidden`.

**Reference implementation to port:** `/Users/Mahesh.Sangawar/personal/tesserix-new/secret-service/apps/api/internal/bao/client.go`. Read it first. It is production code that already handles the hard parts: a mutex-guarded lazy re-login that renews when the token is within a minute of expiry, a `Health` check reporting initialised/unsealed, and error translation (404, 403, KV v2 CAS 400).

Port it; do not redesign it. Adapt only:
- package name and import path
- errors to this repo's conventions — return `bao.ErrNotFound` / `bao.ErrForbidden` sentinels, and do not import any `secret-service` package
- drop anything secret-service-specific this repo has no use for

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/openbao/openbao/api/v2@v2.6.0
```

Pin v2.6.0 — the version secret-service uses, so the estate stays on one client version.

- [ ] **Step 2: Write the failing tests**

Use `httptest` so no OpenBao is needed. Create `internal/bao/client_test.go` covering:

```go
// Login is lazy and cached: two calls inside the token's lease produce
// exactly ONE login request. Count requests to /v1/auth/kubernetes/login.
func TestClient_ReusesTokenWithinLease(t *testing.T)

// A token within a minute of expiry triggers re-login rather than being used.
func TestClient_RenewsBeforeExpiry(t *testing.T)

// A 403 surfaces as ErrForbidden, not an opaque error. The storefront role is
// EXPECTED to hit this on write, so it must be distinguishable from a
// transient failure. Assert with errors.Is.
func TestClient_TranslatesForbidden(t *testing.T)

// A 404 surfaces as ErrNotFound so callers can tell "never written" from
// "backend broken".
func TestClient_TranslatesNotFound(t *testing.T)

// A missing ServiceAccount token file is a clear error, not a panic.
func TestClient_MissingServiceAccountToken(t *testing.T)
```

- [ ] **Step 3: Run and confirm failure**

Run: `go test ./internal/bao/`
Expected: FAIL — package or symbols undefined.

- [ ] **Step 4: Implement by porting**

- [ ] **Step 5: Run and confirm pass**

Run: `go test ./internal/bao/ -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/bao/ go.mod go.sum
git commit -m "feat(bao): port the OpenBao Kubernetes-auth client from secret-service"
```

---

### Task 3: `BaoClient` over KV v2

**Files:**
- Create: `internal/carriersecrets/bao.go`
- Test: `internal/carriersecrets/bao_test.go`

**Interfaces:**
- Consumes: `bao.Client` (Task 2), `ParseBaoReference` (Task 1), the existing `SecretClient` interface (`gcp.go:18`) and `ErrSecretNotFound` sentinel.
- Produces: `NewBaoClient(c *bao.Client, mount string) *BaoClient`, satisfying `SecretClient`. Task 4 stores it as a `SecretClient`.

`BaoClient` implements the **existing** `SecretClient` interface rather than a parallel one, so `ChainStore` treats both backends uniformly:

- `CreateOrAddVersion(ctx, name, payload)` — `name` is the logical path from `ParseBaoReference`. Writes `<mount>/data/<rest>`; KV v2 creates the path or appends a version, matching GCP's create-or-add semantics.
- `AccessLatest(ctx, name)` — reads `<mount>/data/<rest>` and returns the stored value. Maps not-found to the existing `ErrSecretNotFound`, so `ChainStore` needs no backend-specific error handling.
- `DeleteSecret(ctx, name)` — deletes `<mount>/metadata/<rest>`, removing ALL versions. **Not** the `data/` soft delete, which leaves recoverable plaintext and would not match GCP's `DeleteSecret`. Not-found is success, matching GCP.

- [ ] **Step 1: Write the failing tests**

```go
// Round-trip through a fake OpenBao HTTP server.
func TestBaoClient_CreateAddAccess(t *testing.T)

// Writing twice at one path is a new version, not an error, and a subsequent
// read returns the SECOND value.
func TestBaoClient_SecondWriteIsNewVersion(t *testing.T)

// A read of a path never written maps to ErrSecretNotFound, so ChainStore can
// distinguish it from a backend outage.
func TestBaoClient_MissingMapsToErrSecretNotFound(t *testing.T)

// Destroy must hit the METADATA path (removing all versions), not the data
// path (a recoverable soft delete). Assert the URL the client called.
func TestBaoClient_DeleteUsesMetadataPath(t *testing.T)

// Not-found on delete is success, matching GCP's idempotent DeleteSecret.
func TestBaoClient_DeleteNotFoundIsSuccess(t *testing.T)
```

`TestBaoClient_DeleteUsesMetadataPath` is the important one: a soft delete would look like it worked while leaving the credential recoverable.

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/carriersecrets/ -run TestBaoClient`
Expected: FAIL — `undefined: NewBaoClient`.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run and confirm pass**

Run: `go test ./internal/carriersecrets/ -run TestBaoClient -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/carriersecrets/bao.go internal/carriersecrets/bao_test.go
git commit -m "feat(carriersecrets): add a KV v2 backend satisfying SecretClient"
```

---

### Task 4: `ChainStore`

The heart of the phase. Routing is by reference prefix only — never by a mode flag.

**Files:**
- Create: `internal/carriersecrets/chain.go`
- Test: `internal/carriersecrets/chain_test.go`

**Interfaces:**
- Consumes: `SecretClient` (both backends), `crypto.Encryptor`, `Scope`, the Task 1 reference helpers.
- Produces: `type Backend string` with `BackendBao`/`BackendGCP`; `type CounterFn func(label string, increment int64)`; `ChainConfig{Bao, GCP SecretClient; Encryptor crypto.Encryptor; Primary Backend; GCPProjectID, GCPPrefix string; Counter CounterFn}`; `NewChainStore(ChainConfig) *ChainStore` satisfying `Store` and `Rewrapper`. Task 5 wraps it; Task 7 constructs it.

Behaviour:

| Case | Behaviour |
| --- | --- |
| `Put`, primary = Bao | Write via Bao, return `FormatBaoReference(scope)`. On error, **return the error** — never fall back to GCP. |
| `Put`, primary = GCP | Existing GCP behaviour, returns `gsm://…`. |
| `Get("bao://…")` | Bao only. **Never** falls back to GCP — the value is not there. |
| `Get("gsm://…")` | GCP. Increment the fallback counter when primary is Bao. |
| `Get("noop:"/"aes:")` | Encryptor. |
| `Get("")` | Returns `("", nil)`, matching `HybridStore`. |
| `Get(unknown prefix)` | Returns a clear error naming the prefix — never silently treats it as plaintext. |
| `Destroy` | Routes by prefix. Inline references are a no-op, as today. |
| `MaybeRewrap` | Returns a new reference when the old one is NOT already in the primary's format and a rewrap happened; `("", false)` otherwise. |

- [ ] **Step 1: Write the failing tests**

```go
// Table-driven routing: each prefix reaches exactly the right backend and no
// other. Use recording fakes and assert the OTHER backends saw zero calls.
func TestChainStore_GetRoutesByPrefix(t *testing.T)

// The unknown-prefix case must error, not fall through to "treat as plaintext".
func TestChainStore_GetUnknownPrefixErrors(t *testing.T)

// An empty reference is an empty plaintext, matching HybridStore.
func TestChainStore_GetEmptyReference(t *testing.T)

// Put with primary=Bao returns a bao:// reference; the GCP backend is untouched.
func TestChainStore_PutPrimaryBao(t *testing.T)

// THE critical one: when OpenBao is down, Put FAILS and must not write to GCP.
// A silent write fallback would mint gsm:// references after cutover and make
// the phase-5 "fallback counter is zero" evidence a lie.
func TestChainStore_PutDoesNotFallBackOnBaoError(t *testing.T)

// A bao:// read against a failing OpenBao must NOT try GCP — the value is not there.
func TestChainStore_BaoReadDoesNotFallBackToGCP(t *testing.T)

// Reading a gsm:// row while primary is Bao increments the fallback counter.
// Without this counter phase 5 has no evidence and GCP SM is never retired.
func TestChainStore_GSMReadIncrementsFallbackCounter(t *testing.T)

// Reading a bao:// row must NOT increment it.
func TestChainStore_BaoReadDoesNotIncrementFallbackCounter(t *testing.T)

// MaybeRewrap upgrades gsm:// -> bao:// when primary is Bao...
func TestChainStore_MaybeRewrapUpgradesGSM(t *testing.T)

// ...and is a no-op for a reference already in the primary's format.
func TestChainStore_MaybeRewrapNoopForBaoRef(t *testing.T)
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/carriersecrets/ -run TestChainStore`
Expected: FAIL — `undefined: NewChainStore`.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run and confirm pass**

Run: `go test ./internal/carriersecrets/ -run TestChainStore -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/carriersecrets/chain.go internal/carriersecrets/chain_test.go
git commit -m "feat(carriersecrets): add ChainStore routing reads by reference prefix"
```

---

### Task 5: Cache decorator

**Files:**
- Create: `internal/carriersecrets/cache.go`
- Test: `internal/carriersecrets/cache_test.go`

**Interfaces:**
- Consumes: `Store`, `CounterFn` (Task 4).
- Produces: `NewCachingStore(inner Store, ttl time.Duration, clock func() time.Time, counter CounterFn) *CachingStore` satisfying `Store` and forwarding `Rewrapper`. Task 7 constructs it with `60 * time.Second` and `time.Now`.

A decorator, not logic inside `ChainStore` — independently testable and omittable in tests. `clock` is injected so tests control expiry without sleeping.

Behaviour: cache `Get` results by reference for `ttl`. On an inner error, serve a stale entry if one exists (incrementing the stale counter) rather than failing. `Put` and `Destroy` **invalidate** that reference's entry and pass through.

- [ ] **Step 1: Write the failing tests**

```go
// A second Get inside the TTL does not reach the inner store.
func TestCachingStore_HitWithinTTL(t *testing.T)

// After the TTL, the inner store is consulted again.
func TestCachingStore_ExpiresAfterTTL(t *testing.T)

// Stale-on-error: inner fails, a stale entry exists, the stale value is served
// and the stale counter increments.
func TestCachingStore_ServesStaleOnError(t *testing.T)

// With no cached entry, an inner error propagates — stale-on-error must not
// become "swallow all errors".
func TestCachingStore_ErrorPropagatesWithoutStale(t *testing.T)

// Put invalidates, so a rotation is visible immediately to the writer rather
// than waiting out the TTL.
func TestCachingStore_PutInvalidates(t *testing.T)

// Destroy invalidates — a destroyed credential must not keep resolving.
func TestCachingStore_DestroyInvalidates(t *testing.T)

// Concurrent Gets are safe. Run with -race.
func TestCachingStore_ConcurrentGets(t *testing.T)
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./internal/carriersecrets/ -run TestCachingStore`
Expected: FAIL — `undefined: NewCachingStore`.

- [ ] **Step 3: Implement**

- [ ] **Step 4: Run and confirm pass, including the race detector**

Run: `go test ./internal/carriersecrets/ -run TestCachingStore -race -v`
Expected: all PASS, no race reports.

- [ ] **Step 5: Commit**

```bash
git add internal/carriersecrets/cache.go internal/carriersecrets/cache_test.go
git commit -m "feat(carriersecrets): add a TTL cache decorator with stale-on-error reads"
```

---

### Task 6: `FakeBao`

**Files:**
- Create: `internal/carriersecrets/fakebao.go`
- Test: `internal/carriersecrets/fakebao_test.go`

**Interfaces:**
- Produces: `NewFakeBao() *FakeBao` satisfying `SecretClient`, mirroring the existing `FakeClient` in `fake.go`.

- [ ] **Step 1: Read the existing fake**

Read `internal/carriersecrets/fake.go` and match its structure, naming and strictness.

- [ ] **Step 2: Write the failing test**

```go
// The fake must behave like the real backend on the contract ChainStore
// depends on: create-or-version, not-found mapping, delete removes.
func TestFakeBao_SatisfiesSecretClientContract(t *testing.T)
```

- [ ] **Step 3: Run and confirm failure**

Run: `go test ./internal/carriersecrets/ -run TestFakeBao`
Expected: FAIL — `undefined: NewFakeBao`.

- [ ] **Step 4: Implement**

- [ ] **Step 5: Run and confirm pass**

Run: `go test ./internal/carriersecrets/ -run TestFakeBao -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/carriersecrets/fakebao.go internal/carriersecrets/fakebao_test.go
git commit -m "test(carriersecrets): add an in-memory OpenBao fake"
```

---

### Task 7: Config and wiring

**Files:**
- Modify: `pkg/config/config.go` (NOT `internal/config` — the config package lives under `pkg/`, see line 253)
- Modify: `cmd/marketplace-api/main.go` (carrier-secret store construction, from line 394)
- Test: `pkg/config/config_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: a wired `Store` — `NewCachingStore(NewChainStore(...), 60*time.Second, time.Now, counter)`.

**Extend the EXISTING selector; do not add a parallel one.** `pkg/config/config.go:253` already has:

```go
ShippingSecretStore string `envconfig:"SHIPPING_SECRET_STORE" default:"inline"`
```

`main.go:396` switches on it, today handling `gcpsm` and `inline`. Add a third value, `bao`. A second env var such as `CARRIER_SECRET_PRIMARY` would be two settings that can disagree about which backend is primary — exactly the ambiguity this codebase does not need.

| Mode | Store constructed |
| --- | --- |
| `inline` (default, unchanged) | `NewInlineStore` — local dev, no GCP creds |
| `gcpsm` (unchanged) | `ChainStore` with `Primary: BackendGCP` — reads `gsm://` and inline exactly as `HybridStore` does today |
| `bao` (new) | `ChainStore` with `Primary: BackendBao` — writes `bao://`, still reads `gsm://` and inline |

New settings, all only consulted when the mode is `bao`:

| Env var | Meaning | Default |
| --- | --- | --- |
| `OPENBAO_ADDR` | OpenBao API address | `http://openbao-active.openbao.svc.cluster.local:8200` |
| `OPENBAO_ROLE` | Kubernetes auth role | unset |
| `OPENBAO_KV_MOUNT` | KV v2 mount | `kv` |

**The default is unchanged (`inline`), so merging this PR changes nothing at runtime.** Moving a deployment to `bao` is a one-value, revertible config change — that is the rollback story.

Read `main.go:394` onward first and preserve every existing branch, including the degraded-mode handling (`carrierSecretStoreDegraded`) and the dev path with no GCP credentials. `HybridStore` stays in the codebase until phase 5.

- [ ] **Step 1: Write the failing config tests**

```go
// The default is unchanged: an unset SHIPPING_SECRET_STORE is still "inline",
// so merging this PR cannot alter any deployment's behaviour.
func TestConfig_ShippingSecretStoreDefaultUnchanged(t *testing.T)

// "bao" is accepted as a third valid mode.
func TestConfig_ShippingSecretStoreAcceptsBao(t *testing.T)

// An unknown value is rejected at startup, not silently coerced — a typo must
// not quietly leave the wrong backend primary.
func TestConfig_ShippingSecretStoreRejectsUnknownValue(t *testing.T)

// Selecting bao without OPENBAO_ROLE is a startup error, since login cannot work.
func TestConfig_BaoModeRequiresRole(t *testing.T)
```

- [ ] **Step 2: Run and confirm failure**

Run: `go test ./pkg/config/ -run 'TestConfig_ShippingSecretStore|TestConfig_BaoMode'`
Expected: FAIL.

- [ ] **Step 3: Implement config, then the wiring**

- [ ] **Step 4: Verify the tests pass and the binary builds**

Run: `go test ./pkg/config/ -v -run 'TestConfig_ShippingSecretStore|TestConfig_BaoMode'` then `go build ./...`
Expected: PASS, then a clean build.

- [ ] **Step 5: Verify the default really is a no-op**

Run: `go test ./internal/carriersecrets/ ./pkg/config/`
Expected: `ok` for both. State explicitly in the report that with `SHIPPING_SECRET_STORE` unset or `gcpsm`, the constructed store is behaviourally identical to today's — same reads, same reference formats, same degraded-mode handling.

- [ ] **Step 6: Commit**

```bash
git add pkg/config/config.go pkg/config/config_test.go cmd/marketplace-api/main.go
git commit -m "feat(carriersecrets): add a bao mode to SHIPPING_SECRET_STORE, default unchanged"
```

---

### Task 8: Integration test against a real OpenBao

Unit tests with fakes prove the logic. They do not prove we speak KV v2 correctly — that is what phase 1's probe proved by hand, and this task automates.

**Files:**
- Create: `internal/carriersecrets/bao_integration_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `NewBaoClient`, `bao.New`.

Gate on `TEST_OPENBAO_ADDR` and `TEST_OPENBAO_TOKEN`. **Skip loudly** — `t.Skip` with a message naming the missing variable. A silent pass is a failure mode this repo has already been bitten by.

- [ ] **Step 1: Write the test**

```go
//go:build integration

// Round-trips a real secret through a real OpenBao: write, read, write again
// (new version), read the new value, delete metadata, confirm gone.
func TestBaoClient_RealRoundTrip(t *testing.T)
```

Use a unique path per run (`_test/<uuid>`) so concurrent runs cannot collide, and `t.Cleanup` to delete it.

- [ ] **Step 2: Confirm it skips loudly without the env vars**

Run: `go test -tags=integration ./internal/carriersecrets/ -run TestBaoClient_RealRoundTrip -v`
Expected: `SKIP` with a message naming `TEST_OPENBAO_ADDR`. A silent `ok` with no `=== RUN` line means the gate is wrong.

- [ ] **Step 3: Run it against a real OpenBao**

```bash
kubectl -n openbao port-forward svc/openbao-active 8200:8200 &
TEST_OPENBAO_ADDR=http://127.0.0.1:8200 TEST_OPENBAO_TOKEN=<token> \
  go test -tags=integration ./internal/carriersecrets/ -run TestBaoClient_RealRoundTrip -v
```

Expected: PASS with real `=== RUN` output. If you cannot obtain a token, report that in the task report rather than claiming the step passed — the controller will decide.

- [ ] **Step 4: Commit**

```bash
git add internal/carriersecrets/bao_integration_test.go
git commit -m "test(carriersecrets): integration round-trip against a real OpenBao"
```

---

## Phase 2 Done Criteria

- [ ] `go build ./...`, `go vet`, `gofmt` clean
- [ ] Full `./internal/carriersecrets/` and `./internal/bao/` suites pass; cache tests pass under `-race`
- [ ] Existing integration suites still pass with `TEST_DATABASE_URL` set (a silent skip is not a pass)
- [ ] With `SHIPPING_SECRET_STORE` unset or `gcpsm`, behaviour is provably identical to today — the default stays `inline`
- [ ] The `Store` interface is unchanged and no call site outside the package was edited except `main.go`

Phase 3 (backfill) does not begin until a real credential has been saved and read back in production with `SHIPPING_SECRET_STORE=bao`.
