# Operator scripts

These specs are not tests — they are one-off tools that drive a **live**
system (a real admin host, real accounts, real records). They:

- require credentials supplied by the caller (no credential has a default)
- **do NOT require a host** — and that is the danger. Four of these scripts
  fall back to a **production** host when the corresponding variable is
  unset: `admin-verify.spec.ts:20`, `full-flow.spec.ts:27-28`,
  `remote-audit.spec.ts:20`, `storefront-journey.spec.ts:23`. Run one
  without `ADMIN_BASE_URL` / `STOREFRONT_BASE_URL` explicitly set and it
  points at production, where it will create products, place orders, and
  moderate real records. `admin-setup.spec.ts` and
  `seed-product-images.spec.ts` do default their host to `""` and skip
  instead — that is the exception, not the rule here. Always set the host
  variable, even when you believe the default is what you wanted.
- are never run by CI. `.github/scripts/test-ci-contract.py` asserts that no
  workflow so much as mentions the flags below.
- are gated behind an env flag, one per script:

| Script | Env flag |
|---|---|
| `admin-setup.spec.ts` | `FULL_FLOW=1` |
| `admin-verify.spec.ts` | `FULL_FLOW=1` |
| `full-flow.spec.ts` | `FULL_FLOW=1` |
| `storefront-journey.spec.ts` | `FULL_FLOW=1` |
| `remote-audit.spec.ts` | `ADMIN_AUDIT=1` |
| `seed-product-images.spec.ts` | `SEED_IMAGES=1` |

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
