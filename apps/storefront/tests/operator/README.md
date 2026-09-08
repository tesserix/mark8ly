# Operator scripts

These specs are not tests — they are one-off tools that drive a **live**
system (a real storefront host). They:

- require credentials supplied by the caller (no credential has a default)
- **do NOT require a host** — and that is the danger. `remote-audit.spec.ts:19`
  falls back to a **production** storefront when `STOREFRONT_BASE_URL` is
  unset, so an audit run with no host set crawls production. Always set the
  host variable explicitly. (`layout-blocks.spec.ts` defaults to localhost,
  but it PATCHes a real store's branding through a test-only marketplace-api
  endpoint — point it at a throwaway store, never one that matters.)
- are never run by CI. `.github/scripts/test-ci-contract.py` asserts that no
  workflow so much as mentions the flags below.
- are gated behind an env flag, one per script:

| Script | Env flag |
|---|---|
| `remote-audit.spec.ts` | `STOREFRONT_AUDIT=1` |
| `layout-blocks.spec.ts` | `STOREFRONT_VISUAL_TEST_SLUG` |

## Running one

`playwright.config.ts` pins `testDir: "./tests/e2e"`, so these files are
outside the default collected set — `npx playwright test
tests/operator/<spec>.spec.ts` reports **"No tests found"**. Pass the
operator config, which points `testDir` at this directory:

```
STOREFRONT_AUDIT=1 STOREFRONT_BASE_URL=<storefront host> \
npx playwright test --config=playwright.operator.config.ts remote-audit.spec.ts
```

List what the config collects without running anything:

```
npx playwright test --config=playwright.operator.config.ts --list
```

See each script's own header comment for its full list of required env vars.
