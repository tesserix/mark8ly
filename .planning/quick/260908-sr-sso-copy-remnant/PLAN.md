# The SAML claim that survived #839 (#838)

#838 reports two things and **both are already fixed** on `main` as of #839
(`7c560461`):

- `apps/onboarding/components/marketing/Pricing.tsx:103` and
  `apps/admin/lib/copy/pricing.ts:126` both now read `'SSO (OpenID Connect)'`.
- A self-serve SSO config screen exists at
  `apps/admin/app/(admin)/settings/sso/{page,SSOClient}.tsx`, and
  `lib/copy/sso.ts:85` states plainly that SAML is unsupported.

#838 was split out of #820 and filed from notes taken before #839 landed, so it
describes the pre-#839 state. One surface was genuinely missed:

```
apps/onboarding/public/llms-full.txt:43
  ... 100 webhook endpoints, SSO (SAML/OIDC), priority support ...
```

Served at `mark8ly.com/llms-full.txt` — the machine-readable pricing surface.
So the corrected claim reaches humans reading the pricing page while the stale
one still reaches every agent and crawler that reads `llms-full.txt`, which is
the audience least able to notice the contradiction.

## Why the guard is the actual fix

`apps/onboarding/tests/unit/pricing-surfaces-truth.spec.ts` exists for exactly
this failure: it was written for #564 after plan copy drifted across the same
three surfaces, and it already reads all three, `llms-full.txt` included. It had
no SAML assertion, so #839 could correct two surfaces and leave the third
without anything going red.

Editing line 43 alone fixes today's drift and leaves the mechanism that allowed
it. The assertion is what stops the next one.

## Tasks

### 1. Guard the SAML claim across all three surfaces (RED first)
- [ ] Add a test to `pricing-surfaces-truth.spec.ts` asserting no pricing
      surface advertises SAML, and that each surface naming SSO names
      **OpenID Connect** — the positive half catches the opposite drift, where
      the bullet is deleted outright and Pro silently stops advertising the SSO
      it really does have.
- [ ] Use the existing `copyOnly()` helper on the two TypeScript surfaces. Both
      carry a comment naming SAML in order to explain its removal; scanning raw
      source would make documenting the fix trip the guard enforcing it.
- **Done when** the new test fails on `llms-full.txt` and only on
  `llms-full.txt`, naming the file and the line's claim.

### 2. Correct llms-full.txt
- [ ] `SSO (SAML/OIDC)` -> `SSO (OpenID Connect)`, matching the wording the
      other two surfaces settled on in #839.
- **Done when** `npm test -w @mark8ly/onboarding` is green.

### 3. Fix the indentation #839 left in pricing.ts
- [ ] The comment block at `apps/admin/lib/copy/pricing.ts:126-129` and the
      bullet after it sit at the wrong indent inside the `features` array.
      Cosmetic, same hunk, no behaviour.

## Not in scope

Whether to implement SAML at all. #838's second half asks that as a product
question and the honest `501` plus the "contact us" note make the current state
safe either way. Narrowing the last surface does not pre-empt the answer — if
SAML is later built, all three surfaces and this guard change together, which is
the point of having the guard.
