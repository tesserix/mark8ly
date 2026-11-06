# Temporary deepmerge security fork

Based on the published `deepmerge@4.3.1` CommonJS distribution, with its MIT
license and TypeScript declarations preserved. The root npm override routes
all consumers here; no postinstall mutation is required.

GHSA-gcqx-w48m-w2h4 has no published upstream fix as of 2026-11-06. This fork
rejects `__proto__` keys when copying either the target or the source, including
when the target has a null prototype. All other merge behavior is unchanged.
The `4.3.1-patched.1` version and package description identify this as a locally
maintained fork rather than a published upstream fix. Root dependency references
in the override keep every transitive consumer on the same local package.

Run `node --test vendor/deepmerge/security.test.cjs` from the repository root.
Two prototype-poisoning tests fail against the original distribution.
Replace the override with an upstream release once it fixes both paths and
passes these regression and API compatibility tests.

After refreshing the lockfile, run `npm run test:security` with the mobile
workspace installed. This also verifies the actual Expo and Metro consumers:
npm can otherwise resolve local override paths relative to a transitive
parent, producing broken links despite a clean audit.
