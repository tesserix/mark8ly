# OpenFGA Authorization Model

Minimal model for the onboarding-only first slice. Grows when admin and
storefront port.

## Types

- `user` — anyone who can authenticate
- `tenant` — a Mark8ly merchant tenant
  - `member` — a user who belongs to the tenant
  - `owner` — a user who owns the tenant (set during onboarding completion)

## Tuples written during onboarding

When an onboarding session completes, `platform-api` writes:

```
user:<gip-uid> owner tenant:<tenant-id>
user:<gip-uid> member tenant:<tenant-id>
```

Both tuples are written via the **outbox pattern** in the same DB transaction
that creates the tenant row, so they can never be lost (see
`docs/planning/04-auth-and-authz.md`).

## Loading the model

In dev, the `fga-seed` init container POSTs the model JSON to the OpenFGA
container at startup and writes the resulting store ID into a generated env
file that platform-api and auth-bff read.
