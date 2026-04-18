#!/usr/bin/env bash
# verify-no-raw-status-updates.sh — CI guard ensuring every mutation of
# store_subscriptions.status goes through statemachine.Transition (§17.2).
# Exempts the statemachine package itself and test files.
set -euo pipefail
cd "$(dirname "$0")/.."
hits=$(grep -RnE 'UPDATE[[:space:]]+store_subscriptions[[:space:]]+SET[^;]*status[[:space:]]*=' internal/ \
    | grep -v "_test.go" \
    | grep -v "internal/subscription/statemachine/" \
    || true)
if [ -n "$hits" ]; then
    echo "FAIL: direct store_subscriptions.status UPDATE found — route through statemachine.Transition"
    echo "$hits"
    exit 1
fi
echo "OK: no raw store_subscriptions.status UPDATE outside statemachine package"
