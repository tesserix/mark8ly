# Operator scripts

These specs are not tests — they are one-off tools that drive a **live**
system (a real admin host, real accounts, real records). They:

- require credentials supplied by the caller (no defaults are baked in)
- are never run by CI
- are gated behind an env flag, one per script:

| Script | Env flag |
|---|---|
| `admin-setup.spec.ts` | `FULL_FLOW=1` |
| `admin-verify.spec.ts` | `FULL_FLOW=1` |
| `full-flow.spec.ts` | `FULL_FLOW=1` |
| `storefront-journey.spec.ts` | `FULL_FLOW=1` |
| `remote-audit.spec.ts` | `ADMIN_AUDIT=1` |
| `seed-product-images.spec.ts` | `SEED_IMAGES=1` |

Run one directly with its flag and credentials set, e.g.:

```
FULL_FLOW=1 ADMIN_BASE_URL=<admin host> ADMIN_EMAIL=<admin email> \
ADMIN_PASSWORD=<admin password> npx playwright test tests/operator/admin-setup.spec.ts
```

See each script's own header comment for its full list of required env vars.
