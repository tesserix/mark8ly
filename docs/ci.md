# Continuous integration

Mark8ly owns only its product policy and calls the product-neutral workflows in
`tesserix/tesserix-workflows`. The caller is pinned to one immutable central
revision during validation and to the protected lightweight `v2.1.0` release
after the central contract is accepted.

## Gate structure

The `CI` workflow runs for pull requests, pushes to `main`, and manual
rebuilds. Its stable final check is `CI / CI gate`.

- Five Go modules call `go-ci.yml` in parallel.
- The root npm workspace calls `nextjs-ci.yml` once.
- Secret scanning calls `secret-scan.yml`.
- Pull requests build and scan all seven images with `container-ci.yml`.
- Pushes and manual runs on `main` publish all seven images through
  `container-release.yml`, after the language, secret, and product-policy jobs.
- The product-policy job validates this contract and preserves the
  marketplace-api Secret Manager import choke point.

Every selected job fails closed. The final gate rejects a failed or cancelled
dependency; a deliberately skipped PR-only or main-only capability is allowed.
No reusable call uses `secrets: inherit`.

`.gitleaks-baseline.json` fingerprints the 36 reviewed, pre-existing
synthetic test fixtures and public mobile-client configuration findings. Its
matches and secrets are redacted. A new finding, a changed credential-like
fixture, or an unreviewed baseline entry still fails the central secret scan.
Never regenerate the baseline merely to make a red gate pass.

GitHub currently returns `403` for branch-protection and repository-ruleset
configuration on this private repository's plan. Until that plan supports
enforcement, merge only after every job represented by `CI / CI gate`
succeeds. When enforcement becomes available, make that check required and
require at least one approving review.

## Product-owned inputs

`.github/ci/container-images.json` is the single source of truth for image
names, contexts, Dockerfile targets, source-root labels, smoke checks, and the
Trivy policy file. Go service smoke starts are disabled with port `0` until
their external dependencies can be provided deterministically. The three
Next.js images are smoke-tested on `/api/health`.

The caller maps secrets explicitly:

- `GHCR_PAT` supplies registry and private-package access.
- `NEXT_SERVER_ACTIONS_ENCRYPTION_KEY` is exposed only as the
  `APPLICATION_BUILD_SECRET` BuildKit mount. Its truncated fingerprint
  invalidates the build cache after rotation.
- Eight `NEXT_PUBLIC_*` repository values map to generic public build slots.
  These values are public by definition because Next.js embeds them in browser
  bundles.

The customer tenant ID and Apple Services ID are product-owned public literals
in the image matrix. Secret values must never be added to that file.

## Coverage ratchet

The initial line-coverage floors are measured repository baselines, not the
target:

| Go module       | Initial floor |
| --------------- | ------------: |
| platform-api    |           25% |
| auth-bff        |           31% |
| marketplace-api |           21% |
| otto            |            4% |

Raise a module's floor whenever its measured coverage increases. Do not lower a
floor to merge a change. The shared target is 70% line coverage.

The initial Prettier ratchet covers package manifests, GitHub configuration,
and this CI contract. Application code has substantial pre-existing format
drift and remains protected by lint, typecheck, tests, and builds. Expand the
format paths in a dedicated formatting change; do not weaken the current set.

## Base images, performance, and rollback

All seven Dockerfiles pin Tesserix base images by digest. Docker Dependabot owns
digest updates, so a base-image notification can never silently move a build.
The notification workflow validates the pinning and update path; the actual
change arrives as a reviewable PR and passes the same gate.

The operating target is a PR result within 20 minutes and a complete main image
release within 30 minutes. Language and image legs run in parallel and use
GitHub Actions caches. The cost is GitHub-hosted runner time, GHCR storage for
seven immutable images, and provenance/SBOM attestations; no persistent
infrastructure is added.

Rollback is one Git revert of the product caller or a retarget to the previous
protected central release. Published images are immutable and can be selected
by digest while the workflow rollback is validated.
