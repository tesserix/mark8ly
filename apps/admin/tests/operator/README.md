# Operator scripts

These specs are not tests — they are one-off tools that drive a **live**
system (a real admin host, real accounts, real records). They:

- require credentials supplied by the caller (no credential has a default)
- are never run by CI. `.github/scripts/test-ci-contract.py` asserts that no
  workflow so much as mentions the env flags below.

## Two populations, and the difference matters

**Group A — flag-gated, and four of them default to PRODUCTION.** These came
here in #851.

| Script                        | Env flag        |
| ----------------------------- | --------------- |
| `admin-setup.spec.ts`         | `FULL_FLOW=1`   |
| `admin-verify.spec.ts`        | `FULL_FLOW=1`   |
| `full-flow.spec.ts`           | `FULL_FLOW=1`   |
| `storefront-journey.spec.ts`  | `FULL_FLOW=1`   |
| `remote-audit.spec.ts`        | `ADMIN_AUDIT=1` |
| `seed-product-images.spec.ts` | `SEED_IMAGES=1` |

**They do NOT require a host — and that is the danger.** Four fall back to a
**production** host when the corresponding variable is unset: the `ADMIN_URL`
default in `admin-verify.spec.ts`, the `ADMIN_URL` and `STOREFRONT_URL`
defaults in `full-flow.spec.ts`, the `BASE_URL` default in
`remote-audit.spec.ts`, and the `STOREFRONT_URL` default in
`storefront-journey.spec.ts`. Run one without `ADMIN_BASE_URL` /
`STOREFRONT_BASE_URL` explicitly set and it points at production, where it
will create products, place orders, and moderate real records.
`admin-setup.spec.ts` and `seed-product-images.spec.ts` default their host to
`""` and skip instead — the exception, not the rule. **Always set the host
variable, even when you believe the default is what you wanted.**

**Group B — not flag-gated, and cannot reach production by omission.** These
came here in #858 stage 2. Every host and credential defaults to `""`, so an
unset variable produces an immediate failure against an empty host rather
than a silent write to a live estate. That is why they carry no env flag:
the flag in Group A exists to compensate for a production default these do
not have.

| Script                      | Requires                                                                                                                                              |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `account-shot.spec.ts`      | `STOREFRONT_BASE_URL` (+ a storageState another script produced — see below)                                                                          |
| `checkout-au-shot.spec.ts`  | `STOREFRONT_BASE_URL`, `CUSTOMER_EMAIL`, `CUSTOMER_PASSWORD`                                                                                          |
| `delivery-timeline.spec.ts` | `ADMIN_BASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `TENANT_NAME`, `STOREFRONT_BASE_URL`, `CUSTOMER_EMAIL`, `CUSTOMER_PASSWORD`, `RAZORPAY_KEY_SECRET` |
| `homepage-shot.spec.ts`     | `STOREFRONT_BASE_URL`                                                                                                                                 |
| `image-audit.spec.ts`       | `ADMIN_BASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `TENANT_NAME`, `STOREFRONT_BASE_URL`                                                               |
| `persistence-audit.spec.ts` | `STOREFRONT_BASE_URL`, `CUSTOMER_EMAIL`, `CUSTOMER_PASSWORD`                                                                                          |
| `product-health.spec.ts`    | `STOREFRONT_BASE_URL`                                                                                                                                 |
| `products-sync.spec.ts`     | `ADMIN_BASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `TENANT_NAME`, `STOREFRONT_BASE_URL`                                                               |
| `profile-probe.spec.ts`     | `STOREFRONT_BASE_URL`, `CUSTOMER_EMAIL`, `CUSTOMER_PASSWORD`                                                                                          |
| `purchase-journey.spec.ts`  | `ADMIN_BASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `TENANT_NAME`, `STOREFRONT_BASE_URL`, `CUSTOMER_EMAIL`, `CUSTOMER_PASSWORD`, `RAZORPAY_KEY_SECRET` |
| `shipping-probe.spec.ts`    | `ADMIN_BASE_URL`, `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `TENANT_NAME`                                                                                      |
| `storefront-final.spec.ts`  | `STOREFRONT_BASE_URL`, `STOREFRONT_FINAL_PRODUCT_HANDLE` (skips without the handle)                                                                   |

## Why Group B is here rather than in `tests/e2e/`

Measured for #858, not assumed. A file under `tests/e2e/` claims to be
enforced coverage; these could never be enforced by CI:

- **Six assert nothing at all.** `account-shot`, `checkout-au-shot`,
  `homepage-shot`, `profile-probe`, `shipping-probe` and `storefront-final`
  contain zero `expect()` calls — they capture screenshots or poke a surface.
  A run that asserts nothing exits 0 whatever the system did, which is the
  precise failure #834 exists to prevent.
- **All twelve attach to an existing tenant** via `TENANT_NAME` plus
  caller-supplied credentials, or to a storefront the caller names. The specs
  that remain in `tests/e2e/` build their own tenant with
  `completeOnboarding()` and are therefore self-contained and repeatable.
  That is the dividing line, and it partitions the directory cleanly with no
  judgement calls left over.
- **`account-shot` is not even self-contained among these** — it consumes a
  Playwright storageState that `purchase-journey` or `delivery-timeline` has
  to produce first, into `tests/e2e/.audit/customer-state.json`. All five
  files in that producer/consumer chain now live in this directory together.

**Moving them is not dropping coverage.** The purchase journey, product sync
and delivery timeline still deserve enforced coverage; these particular
scripts are not the vehicle, because a script that needs a real Razorpay
secret and a pre-existing tenant cannot run on a clean CI runner. #858 stage
3 covers those flows with self-contained specs.

## Running one

`playwright.config.ts` pins `testDir: "./tests/e2e"`, so these files are
outside the default collected set — `npx playwright test
tests/operator/<spec>.spec.ts` reports **"No tests found"**. Pass the
operator config, which points `testDir` at this directory:

```
FULL_FLOW=1 ADMIN_BASE_URL=<admin host> ADMIN_EMAIL=<admin email> \
ADMIN_PASSWORD=<admin password> \
npx playwright test --config=playwright.operator.config.ts admin-setup.spec.ts
```

List what the config collects without running anything:

```
npx playwright test --config=playwright.operator.config.ts --list
```

See each script's own header comment for its full list of required env vars.
