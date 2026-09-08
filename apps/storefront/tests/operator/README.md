# Operator scripts

These specs are not tests — they are one-off tools that drive a **live**
system (a real storefront host). They:

- require credentials supplied by the caller (no defaults are baked in)
- are never run by CI
- are gated behind an env flag, one per script:

| Script | Env flag |
|---|---|
| `remote-audit.spec.ts` | `STOREFRONT_AUDIT=1` |
| `layout-blocks.spec.ts` | `STOREFRONT_VISUAL_TEST_SLUG` |

Run one directly with its flag and credentials set, e.g.:

```
STOREFRONT_AUDIT=1 STOREFRONT_BASE_URL=<storefront host> \
npx playwright test tests/operator/remote-audit.spec.ts
```

See each script's own header comment for its full list of required env vars.
