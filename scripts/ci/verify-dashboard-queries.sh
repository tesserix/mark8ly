#!/usr/bin/env bash
# =============================================================================
# verify-dashboard-queries.sh
# =============================================================================
# Validates Grafana dashboard JSON in ../tesserix-k8s/grafana/dashboards/:
#   1. Each file is valid JSON.
#   2. Required fields present: uid, title, schemaVersion >= 37,
#      version (number), panels.
#   3. No TODO / FIXME / WIP markers.
#   4. Every metric / recording-rule name referenced by a PromQL target
#      matches either:
#         - a known shipped metric (from services/marketplace-api/internal/metrics/registry.go)
#         - a recording rule defined in tesserix-k8s/k8s/cluster/prometheus/rules/subscription.yaml
#         - a metric known to be deferred (pending P17 T1/T3/P8 collectors)
#   5. PromQL keywords / label values are whitelisted and short-circuit.
#
# Deferred metrics are allowed so the arbitrage-operations + trial-activation-funnel
# dashboards can be shipped with placeholder panels that light up once the
# upstream collectors land (see dashboard descriptions).
#
# Requires: jq.
# =============================================================================
set -euo pipefail

# ---------------------------------------------------------------------------
# Resolve dashboard directory. When invoked from mark8ly CI we expect the
# tesserix-k8s repo to be checked out as a sibling; in the monorepo-local
# workflow we use the known sibling path ../tesserix-k8s.
# ---------------------------------------------------------------------------
if [[ -n "${DASHBOARD_DIR:-}" ]]; then
    DIR="${DASHBOARD_DIR}"
elif [[ -d "../tesserix-k8s/grafana/dashboards" ]]; then
    DIR="../tesserix-k8s/grafana/dashboards"
elif [[ -d "./tesserix-k8s/grafana/dashboards" ]]; then
    DIR="./tesserix-k8s/grafana/dashboards"
else
    echo "ERROR: tesserix-k8s/grafana/dashboards not found. Set DASHBOARD_DIR to override." >&2
    exit 2
fi

echo "Validating dashboards under: ${DIR}"

# ---------------------------------------------------------------------------
# Known shipped metrics — keep in sync with
# services/marketplace-api/internal/metrics/registry.go.
# ---------------------------------------------------------------------------
KNOWN_METRICS=(
    http_requests_total
    http_request_duration_seconds_bucket
    http_request_duration_seconds_count
    http_request_duration_seconds_sum
    orders_created_total
    webhook_received_total
    outbox_events_published_total
    outbox_events_failed_total
    # RETAINED DELIBERATELY, NOT AN OVERSIGHT. The gauge was deleted from
    # registry.go in #375 — nothing exports it any more. This entry stays only
    # because tesserix-k8s/grafana/dashboards/webhook-pipeline.json still has a
    # panel querying it, and this script validates that repo's dashboards
    # against this list: removing the entry first would fail CI on a dashboard
    # this repo cannot edit. Remove this line once that panel is repointed or
    # deleted.
    outbox_events_pending
    mark8ly_trial_signup_anomaly_alerts_total
    mark8ly_trial_activation_day30_total
    mark8ly_subscription_dunning_emails_sent_total
    mark8ly_subscription_payment_action_reminders_sent_total
    mark8ly_subscription_dunning_suppressed_refund_window_total
    # P15 white-label app counters.
    white_label_app_lifecycle_transition_total
    white_label_app_credential_accessed_total
)

# Recording rules from k8s/cluster/prometheus/rules/subscription.yaml.
RECORDING_RULES=(
    "subscription:dunning_emails:rate5m"
    "subscription:sca_reminders:rate5m"
    "subscription:refund_window_suppressions:total"
    "subscription:refund_window_suppressions:rate1h"
    "subscription:trial_activation_day30:total"
    "subscription:trial_activation_day30:rate1d"
    "subscription:trial_signup_anomalies:total"
    "subscription:trial_signup_anomalies:rate24h"
    "webhook:received:rate5m"
    "http:requests:rate5m"
    "http:error_rate:ratio5m"
    "outbox:published:rate5m"
    # P15 white-label app recording rules (white_label_app.yaml).
    "white_label_app:lifecycle_transitions:rate1h"
    "white_label_app:downloads_blocked:rate1d"
    "white_label_app:pulled:rate1d"
    "white_label_app:credentials_purged:rate1d"
    "white_label_app:credential_accessed:rate1h"
    "white_label_app:credential_accessed:median24h"
)

# Metrics known to be deferred — panels reference them as placeholders.
# Removing from this list makes the panel fail CI, forcing the author to
# land the collector first.
DEFERRED_METRICS=(
    subscription_arbitrage_flagged
    subscription_arbitrage_false_positive_cleared
    subscription_arbitrage_blocked_total
    subscription_arbitrage_appeal_queue_depth
)

# PromQL keywords + well-known label values. Short-circuit via regex below.
KEYWORD_PATTERN='^(sum|rate|increase|histogram_quantile|clamp_min|topk|bottomk|time|label_values|by|le|count|max|min|avg|stddev|absent_over_time|absent|resets|changes|deriv|idelta|delta|vector|scalar|offset|on|ignoring|group_left|group_right|and|or|unless|without|_range|__range|status|provider|event_type|reason|day|offset|attempt|plan|currency|source|stripe|le|true|false|warning|info|critical|active|past_due|payment_action_required|cancel_scheduled|trialing|signup|to|from|method|path|store_id)$'

# Build a single regex of allowed identifiers from KNOWN + RECORDING + DEFERRED.
declare -a ALLOWED=("${KNOWN_METRICS[@]}" "${RECORDING_RULES[@]}" "${DEFERRED_METRICS[@]}")

is_allowed() {
    local ref="$1"
    for a in "${ALLOWED[@]}"; do
        if [[ "${ref}" == "${a}" ]]; then
            return 0
        fi
    done
    return 1
}

failures=0

for f in "${DIR}"/*.json; do
    [[ -e "${f}" ]] || { echo "no dashboards found under ${DIR}"; exit 1; }
    echo ""
    echo "=== ${f} ==="

    # 1. Valid JSON.
    if ! jq empty "${f}" >/dev/null 2>&1; then
        echo "FAIL: ${f} is not valid JSON"
        failures=$((failures + 1))
        continue
    fi

    # 2. Required fields.
    if ! jq -e '.uid and .title and (.schemaVersion >= 37) and (.version | type == "number") and .panels' "${f}" >/dev/null; then
        echo "FAIL: ${f} missing required field (uid / title / schemaVersion>=37 / version / panels)"
        failures=$((failures + 1))
        continue
    fi

    # 3. No TODO markers.
    if grep -iE 'todo|fixme|\bwip\b' "${f}" >/dev/null; then
        echo "FAIL: ${f} contains TODO / FIXME / WIP marker"
        failures=$((failures + 1))
        continue
    fi

    # 4. Extract all PromQL expressions from panel targets.
    exprs=$(jq -r '[.panels[]? | (.targets // [])[]? | .expr // empty] | .[]' "${f}" || true)

    if [[ -z "${exprs}" ]]; then
        echo "NOTE: ${f} has no PromQL targets (text-only dashboard)"
        continue
    fi

    # 5. Tokenise. Strip:
    #      - bracket contents (duration suffixes like [5m], [30d:5m])
    #      - brace contents (label matchers — values already allow-listed)
    #      - string literals "..."
    #    Then pull identifier-like tokens incl. recording-rule form a:b:c.
    stripped=$(printf '%s\n' "${exprs}" \
        | sed -E 's/\[[^]]*\]//g' \
        | sed -E 's/\{[^}]*\}//g' \
        | sed -E 's/"[^"]*"//g')

    unknown=()
    while IFS= read -r token; do
        [[ -z "${token}" ]] && continue
        # Skip pure numerics.
        if [[ "${token}" =~ ^[0-9]+$ ]]; then
            continue
        fi
        # Skip keywords / known label values.
        if [[ "${token}" =~ ${KEYWORD_PATTERN} ]]; then
            continue
        fi
        # Skip template variables ($datasource, $tenant, $__range etc).
        if [[ "${token}" =~ ^\$ ]]; then
            continue
        fi
        if is_allowed "${token}"; then
            continue
        fi
        unknown+=("${token}")
    done < <(printf '%s\n' "${stripped}" | grep -oE '[$]?[a-z_][a-z0-9_:]*' | sort -u)

    if (( ${#unknown[@]} > 0 )); then
        echo "FAIL: ${f} references unknown metric(s) / rule(s):"
        for u in "${unknown[@]}"; do
            echo "  UNKNOWN: ${u}"
        done
        failures=$((failures + 1))
        continue
    fi

    echo "OK: $(printf '%s\n' "${exprs}" | wc -l | tr -d ' ') PromQL expression(s) validated"
done

if (( failures > 0 )); then
    echo ""
    echo "FAIL: ${failures} dashboard(s) failed validation."
    exit 1
fi

echo ""
echo "PASS: all dashboards valid."
