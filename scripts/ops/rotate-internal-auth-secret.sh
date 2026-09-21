#!/usr/bin/env bash
#
# Rotate MARKETPLACE_INTERNAL_AUTH_SECRET.
#
# WHY THIS EXISTS
#   auth-bff's /internal/* routes (mint-session, user delete, display-name,
#   linked providers) are guarded by ONE shared static header compared against
#   this secret. Whoever holds it can mint a valid session for any user_id in
#   any tenant. Until tesserix-k8s#1063 those routes answered 401 — not 404 —
#   from the public internet, so the secret must be treated as exposed.
#
# THE TRAP THIS SCRIPT EXISTS TO AVOID
#   SEVEN deployments read this same secret. Both sides of every internal call
#   compare the same string, so a partial rollout breaks mint-session, user
#   deletion and display-name lookups for as long as the two halves disagree.
#   They must all be restarted, and the rollouts must be allowed to finish.
#
# WHAT IT DOES NOT DO
#   It never prints the secret, old or new. It makes no change until you
#   confirm. Run it from a machine with gcloud + kubectl already authenticated
#   against the prod project and cluster.
#
set -euo pipefail

PROJECT="${PROJECT:-tesseracthub-480811}"
NAMESPACE="${NAMESPACE:-mark8ly}"
GCP_SECRET="${GCP_SECRET:-prod-mark8ly-marketplace-api-internal-auth}"
K8S_SECRET="${K8S_SECRET:-mark8ly-marketplace-api-internal-auth}"
EXTERNAL_SECRET="${EXTERNAL_SECRET:-mark8ly-marketplace-api-internal-auth}"

# Every deployment whose pods read this secret. Verified 2026-09-21 by
# inspecting each deployment's env for a secretKeyRef to $K8S_SECRET.
# Re-derive with:
#   kubectl get deploy -n mark8ly -o json | jq -r '.items[] | . as $d |
#     .spec.template.spec.containers[].env[]? |
#     select(.valueFrom.secretKeyRef.name=="mark8ly-marketplace-api-internal-auth") |
#     $d.metadata.name' | sort -u
CONSUMERS=(
  mark8ly-admin
  mark8ly-auth-bff
  mark8ly-marketplace-api-admin
  mark8ly-marketplace-api-storefront
  mark8ly-mcp
  mark8ly-platform-api
  mark8ly-storefront
)

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
die() { printf '\nERROR: %s\n' "$*" >&2; exit 1; }

command -v gcloud  >/dev/null || die "gcloud not on PATH"
command -v kubectl >/dev/null || die "kubectl not on PATH"

say "Preflight"
echo "  project     : $PROJECT"
echo "  namespace   : $NAMESPACE"
echo "  gcp secret  : $GCP_SECRET"
echo "  k8s secret  : $K8S_SECRET"
echo "  consumers   : ${#CONSUMERS[@]} deployments"

gcloud secrets describe "$GCP_SECRET" --project "$PROJECT" >/dev/null 2>&1 \
  || die "secret $GCP_SECRET not found in $PROJECT"

# Confirm the consumer list still matches reality. If a deployment was added
# since this script was written, restarting the old list leaves it stranded on
# the old secret and its internal calls will fail.
say "Verifying the consumer list against the cluster"
ACTUAL=$(kubectl get deploy -n "$NAMESPACE" -o json \
  | python3 -c '
import sys, json
d = json.load(sys.stdin)
names = set()
for it in d["items"]:
    for c in it["spec"]["template"]["spec"]["containers"]:
        for e in c.get("env", []):
            ref = e.get("valueFrom", {}).get("secretKeyRef", {})
            if ref.get("name") == sys.argv[1]:
                names.add(it["metadata"]["name"])
print("\n".join(sorted(names)))
' "$K8S_SECRET")

EXPECTED=$(printf '%s\n' "${CONSUMERS[@]}" | sort)
if [[ "$ACTUAL" != "$EXPECTED" ]]; then
  echo "  --- cluster says ---"; echo "$ACTUAL"
  echo "  --- script expects ---"; echo "$EXPECTED"
  die "consumer list is stale. Update CONSUMERS before rotating."
fi
echo "  consumer list matches."

say "This will rotate the secret and restart ${#CONSUMERS[@]} deployments"
printf '%s\n' "${CONSUMERS[@]}" | sed 's/^/    /'
read -r -p $'\nType ROTATE to proceed: ' CONFIRM
[[ "$CONFIRM" == "ROTATE" ]] || die "aborted"

say "1/5 Adding a new secret version"
# 48 bytes of urandom, base64 -> 64 chars. Never echoed.
NEW=$(head -c 48 /dev/urandom | base64 | tr -d '\n')
printf '%s' "$NEW" | gcloud secrets versions add "$GCP_SECRET" \
  --project "$PROJECT" --data-file=- >/dev/null
unset NEW
VERSION=$(gcloud secrets versions list "$GCP_SECRET" --project "$PROJECT" \
  --filter="state=enabled" --sort-by=~createTime --limit=1 --format="value(name)")
echo "  new enabled version: $VERSION"

say "2/5 Forcing an ExternalSecret resync"
# ESO's refreshInterval is 1h; this annotation makes it reconcile now.
kubectl annotate externalsecret "$EXTERNAL_SECRET" -n "$NAMESPACE" \
  force-sync="$(date +%s)" --overwrite >/dev/null
for _ in $(seq 1 30); do
  STATUS=$(kubectl get externalsecret "$EXTERNAL_SECRET" -n "$NAMESPACE" \
    -o jsonpath='{.status.conditions[?(@.type=="Ready")].reason}' 2>/dev/null || true)
  [[ "$STATUS" == "SecretSynced" ]] && break
  sleep 2
done
[[ "$STATUS" == "SecretSynced" ]] || die "ExternalSecret did not reach SecretSynced (last: ${STATUS:-unknown})"
echo "  ExternalSecret: SecretSynced"

# Prove the k8s Secret actually changed, without printing it.
HASH=$(kubectl get secret "$K8S_SECRET" -n "$NAMESPACE" \
  -o jsonpath='{.data.INTERNAL_AUTH_SECRET}' | shasum -a 256 | cut -c1-12)
echo "  k8s secret fingerprint: $HASH"

say "3/5 Restarting all consumers together"
# Restart every deployment before waiting on any of them: the window where
# halves disagree must be as short as possible.
for d in "${CONSUMERS[@]}"; do
  kubectl rollout restart "deploy/$d" -n "$NAMESPACE" >/dev/null
  echo "  restarted $d"
done

say "4/5 Waiting for every rollout to finish"
FAILED=()
for d in "${CONSUMERS[@]}"; do
  if kubectl rollout status "deploy/$d" -n "$NAMESPACE" --timeout=5m >/dev/null 2>&1; then
    echo "  ok      $d"
  else
    echo "  FAILED  $d"
    FAILED+=("$d")
  fi
done
if (( ${#FAILED[@]} )); then
  die "these did not roll out: ${FAILED[*]} — internal calls may be failing. Investigate before walking away."
fi

say "5/5 Verification"
echo -n "  /internal/mint-session should be 404 (blocked at the edge): "
curl -s -o /dev/null -w '%{http_code}\n' --max-time 15 \
  -X POST https://auth.mark8ly.com/internal/mint-session || echo "probe failed"
echo -n "  /auth/session should still answer (401 unauthenticated):    "
curl -s -o /dev/null -w '%{http_code}\n' --max-time 15 \
  https://auth.mark8ly.com/auth/session || echo "probe failed"

cat <<EOF

Done. Still to check by hand — these exercise the shared secret end to end and
a mismatch only shows up here:
  - admin login (auth-bff -> marketplace-api mint-session)
  - a storefront page that reads linked providers
  - an admin action that deletes a user

If any of those fail, the halves disagree: re-run step 2 and confirm every
pod is on the new fingerprint.

The previous secret version is still enabled. Once you have verified the
above, disable it:
  gcloud secrets versions disable <previous> --secret=$GCP_SECRET --project=$PROJECT
Leaving it enabled is what allowed the 2026-09-03 incident to run a live key
for six minutes.
EOF
