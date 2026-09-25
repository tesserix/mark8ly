#!/usr/bin/env bash
# Copy a secret from GCP Secret Manager into a GitHub Actions repo secret.
#
# The value is piped straight from gcloud into gh. It is never echoed, never
# written to a file, and never lands in shell history — which is the whole
# point of doing this with a script rather than by hand.
#
#   ./scripts/ops/sync-secret-gcp-to-github.sh \
#       prod-mark8ly-sentry-auth-token SENTRY_AUTH_TOKEN
#
# Defaults suit this repo; override with GCP_PROJECT / GITHUB_REPO.

set -euo pipefail

GCP_SECRET="${1:?usage: $0 <gcp-secret-name> <github-secret-name>}"
GH_SECRET="${2:?usage: $0 <gcp-secret-name> <github-secret-name>}"
GCP_PROJECT="${GCP_PROJECT:-tesseracthub-480811}"
GITHUB_REPO="${GITHUB_REPO:-tesserix/mark8ly}"

command -v gcloud >/dev/null || { echo "gcloud not found" >&2; exit 1; }
command -v gh >/dev/null     || { echo "gh not found" >&2; exit 1; }

# Fail before touching GitHub if the source does not exist, so a typo cannot
# quietly create an empty GitHub secret — an empty secret is worse than none,
# because it shadows values the build would otherwise resolve elsewhere.
if ! gcloud secrets describe "$GCP_SECRET" --project "$GCP_PROJECT" >/dev/null 2>&1; then
  echo "no such secret in $GCP_PROJECT: $GCP_SECRET" >&2
  exit 1
fi

bytes=$(gcloud secrets versions access latest \
          --secret="$GCP_SECRET" --project="$GCP_PROJECT" | wc -c | tr -d ' ')
if [ "$bytes" -eq 0 ]; then
  echo "$GCP_SECRET is empty — refusing to write $GH_SECRET" >&2
  exit 1
fi

# Strip trailing newlines. `gcloud secrets versions add --data-file=-` keeps
# whatever you typed, and pressing Enter before Ctrl-D stores a trailing "\n".
# It survives every display and every length check that trims, then produces a
# malformed Authorization header whose error reads as an INVALID token rather
# than a malformed one. This project has already lost time to exactly that,
# with a Stripe key. Strip at the point of use so the stored copy cannot leak
# the problem onward.
gcloud secrets versions access latest --secret="$GCP_SECRET" --project="$GCP_PROJECT" \
  | perl -0777 -pe 's/\s+\z//' \
  | gh secret set "$GH_SECRET" --repo "$GITHUB_REPO"

clean=$(gcloud secrets versions access latest --secret="$GCP_SECRET" --project="$GCP_PROJECT" \
          | perl -0777 -pe 's/\s+\z//' | wc -c | tr -d ' ')
echo "set $GH_SECRET on $GITHUB_REPO from $GCP_SECRET ($clean bytes, $((bytes - clean)) trailing byte(s) stripped)"
echo "verify:  gh secret list --repo $GITHUB_REPO | grep $GH_SECRET"
