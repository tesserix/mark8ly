#!/usr/bin/env bash
# Verifies #358 (POST /admin/billing/trials/{store_id}/extend moving a
# card-backed trial's trial_end in Stripe) against Stripe TEST MODE.
#
# WHY THIS SCRIPT EXISTS: production holds ZERO store_subscriptions rows, so
# a production deploy can only prove the route is mounted and refuses
# unsigned callers — it cannot exercise the Stripe path at all. This script
# creates a real test-mode trialing subscription, gives you the sequence to
# drive the actual endpoint against it, and then reads Stripe back to check
# what really happened. Never run this with a live (sk_live_) key — see the
# guard below.
#
## Verifying #358 against Stripe test mode
#
# 1. Obtain a Stripe TEST secret key (starts with sk_test_). Options:
#      - Stripe dashboard, test mode, Developers -> API keys, or
#      - from GCP Secret Manager IF a test-mode secret is stored there, e.g.:
#          gcloud secrets versions access latest --secret=<some-test-secret-name>
#        (the CONTROLLER decides where the key comes from; this script never
#        fetches it itself)
#    Export it: export STRIPE_TEST_KEY=sk_test_...
#
# 2. Run the setup half:
#      ./scripts/verify-358-stripe.sh setup
#    This prints a subscription id (sub_...), its price id, its trial_end
#    and its billing_cycle_anchor (which equals trial_end at creation).
#
# 3. Seed a local store_subscriptions row pointing at that subscription
#    (see the report for the exact INSERT — you need an existing stores(id)
#    row, status='trialing', stripe_subscription_id = the sub_... above).
#
# 4. Issue a SIGNED POST to
#      /api/v1/platform/admin/billing/trials/<store_id>/extend
#    with an Idempotency-Key, reason_code=goodwill and a trial_ends_at
#    ~60 days out (see internal/handlers/platformadmin/signature.go and the
#    report for the exact HMAC recipe).
#
# 5. Re-run this script in check mode with the subscription id, the exact
#    unix trial_ends_at you sent, and the ORIGINAL anchor + price id printed
#    in step 2:
#      ./scripts/verify-358-stripe.sh check sub_XXXX <expected_unix> <original_anchor_unix> <original_price_id>
#    This prints PASS/FAIL for all four claims: trial_end moved to the exact
#    integer, billing_cycle_anchor ALSO moved there, the price id is
#    unchanged, and status is still trialing.
#
set -euo pipefail

# --- Guard: refuse anything that is not a test-mode key. This is the whole
# reason it is safe to run this script at all, so it runs FIRST, before any
# other line of this script executes. -----------------------------------
if [ -z "${STRIPE_TEST_KEY:-}" ]; then
  cat >&2 <<'EOF'
REFUSING: STRIPE_TEST_KEY is not set.

This script reads the key from the STRIPE_TEST_KEY environment variable —
it does not fetch one itself. Obtain a Stripe TEST-mode secret key
(starts with sk_test_) and export it, e.g.:

  export STRIPE_TEST_KEY=sk_test_...

or, if a test-mode secret is stored in GCP Secret Manager under some name
you control:

  export STRIPE_TEST_KEY="$(gcloud secrets versions access latest --secret=<test-secret-name>)"

Then re-run this script.
EOF
  exit 1
fi

case "$STRIPE_TEST_KEY" in
  sk_test_*) ;;
  *)
    echo "REFUSING: STRIPE_TEST_KEY does not start with sk_test_ — this script must never touch live billing" >&2
    exit 1
    ;;
esac

KEY="$STRIPE_TEST_KEY"

# The key is fed to curl via --config (reading a "user = ..." line from
# stdin) rather than -u, because -u would put the credential in this
# process's argv, where any user on the machine can read it via `ps`.
api() {
  local path="$1"; shift
  printf 'user = "%s:"\n' "$KEY" | curl -sS --config - "https://api.stripe.com/v1/$path" "$@"
}

cmd="${1:-setup}"

run_setup() {
  echo "==> creating a test-mode customer + trialing subscription" >&2
  CUS=$(api customers -d "description=mark8ly-358-verify" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
  PM=$(api payment_methods -d "type=card" -d "card[token]=tok_visa" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
  api "payment_methods/$PM/attach" -d "customer=$CUS" >/dev/null
  PRICE=$(api prices -d "unit_amount=2900" -d "currency=gbp" -d "recurring[interval]=month" \
          -d "product_data[name]=mark8ly-358-verify" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

  TRIAL_END=$(( $(date +%s) + 10*24*3600 ))
  SUB_JSON=$(api subscriptions -d "customer=$CUS" -d "items[0][price]=$PRICE" \
             -d "trial_end=$TRIAL_END" -d "proration_behavior=none")
  SUB=$(echo "$SUB_JSON" | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
  ANCHOR=$(echo "$SUB_JSON" | python3 -c 'import json,sys;print(json.load(sys.stdin)["billing_cycle_anchor"])')

  echo "customer=$CUS"
  echo "price=$PRICE"
  echo "subscription=$SUB"
  echo "trial_end=$TRIAL_END"
  echo "billing_cycle_anchor=$ANCHOR"

  cat >&2 <<NEXT

==> now, by hand:
  1. seed a local store_subscriptions row for a known store id with
     stripe_subscription_id = $SUB, status = 'trialing'
  2. POST /api/v1/platform/admin/billing/trials/<store_id>/extend
     with a signed request, an Idempotency-Key, reason_code=goodwill,
     and trial_ends_at = 60 days out
  3. re-run this script with:
       $0 check $SUB <expected_unix> $ANCHOR $PRICE

NEXT
}

run_check() {
  local sub="$1" exp="$2" orig_anchor="$3" orig_price="$4"
  api "subscriptions/$sub" | python3 -c '
import json,sys
d = json.load(sys.stdin)
exp = int(sys.argv[1])
orig_anchor = int(sys.argv[2])
orig_price = sys.argv[3]

def result(ok):
    return "PASS" if ok else "FAIL"

trial_end = d["trial_end"]
anchor = d["billing_cycle_anchor"]
status = d["status"]
price_id = d["items"]["data"][0]["price"]["id"]

print("[%s] trial_end            = %s (expected %s)" % (result(trial_end == exp), trial_end, exp))
print("[%s] billing_cycle_anchor = %s (expected %s) -- anchor moved from %s to %s" %
      (result(anchor == exp), anchor, exp, orig_anchor, anchor))
print("[%s] status               = %s (expected trialing)" % (result(status == "trialing"), status))
print("[%s] price id             = %s (expected unchanged, original %s)" %
      (result(price_id == orig_price), price_id, orig_price))
' "$exp" "$orig_anchor" "$orig_price"
}

case "$cmd" in
  setup)
    run_setup
    ;;
  check)
    if [ $# -lt 5 ]; then
      echo "usage: $0 check <sub_id> <expected_unix> <original_anchor_unix> <original_price_id>" >&2
      exit 1
    fi
    run_check "$2" "$3" "$4" "$5"
    ;;
  *)
    echo "usage: $0 {setup|check <sub_id> <expected_unix> <original_anchor_unix> <original_price_id>}" >&2
    exit 1
    ;;
esac
