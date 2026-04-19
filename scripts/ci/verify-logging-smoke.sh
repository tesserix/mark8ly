#!/usr/bin/env bash
# =============================================================================
# Log-shipping PII smoke guard (P17 Task 12)
# =============================================================================
# Static grep that fails CI if any Go service's internal package code uses
# fmt.Print* with PII-adjacent field names (tenant_id, email, trace_id, …).
#
# Scope: services/**/internal/**/*.go (non-test files only).
# CLI binaries (cmd/**) and test helpers are intentionally excluded — they
# have legitimate uses of fmt.Println/Printf for human-readable output.
#
# Rationale: structured loggers (slog, logrus) emit JSON that Cloud Logging
# indexes. fmt.Printf in internal service code bypasses labelled fields and
# may interpolate sensitive identifiers directly into unstructured log lines.
#
# Checks:
#   1. fmt.Print* in internal/**/*.go (blanket, non-test files)
#   2. fmt.Print* with a PII-adjacent keyword anywhere in services (incl. cmd)
#
# False positive escape hatch:
#   Add  //nolint:logging-smoke  on the offending line after review.
#
# Exit codes: 0 = clean, 1 = violations found.
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
SERVICES_DIR="${REPO_ROOT}/services"

if [[ ! -d "${SERVICES_DIR}" ]]; then
    echo "SKIP: services directory not found at ${SERVICES_DIR}" >&2
    exit 0
fi

FAILED=0

# ---- Check 1: fmt.Print* in internal/**/*.go (non-test, non-cmd) ----------
echo "=== Check 1: fmt.Print* in internal service code (non-test files) ==="

# Find all internal/*.go files under services/ that are not test files.
# The path must contain /internal/ to exclude cmd/ binaries.
PRINT_VIOLATIONS=$(
    grep -rn --include="*.go" \
        -E 'fmt\.(Print|Printf|Println|Fprint|Fprintf|Fprintln)\(' \
        "${SERVICES_DIR}" \
        | grep '/internal/' \
        | grep -v '_test\.go:' \
        | grep -v '//nolint:logging-smoke' \
        | grep -v '// nolint:logging-smoke' \
        || true
)

if [[ -n "${PRINT_VIOLATIONS}" ]]; then
    echo "FAIL: fmt.Print* calls found in internal service code."
    echo "Use slog or logrus for structured logging instead."
    echo ""
    echo "${PRINT_VIOLATIONS}"
    echo ""
    echo "To suppress a legitimate use, add  //nolint:logging-smoke  on the same line."
    FAILED=1
else
    echo "PASS: no fmt.Print* calls in internal service code."
fi

# ---- Check 2: PII-adjacent fields in any fmt.Print* call -------------------
echo ""
echo "=== Check 2: PII-adjacent fields in fmt.Print* calls (all service files) ==="

PII_PATTERN='(email|tenant_id|trace_id|user_id|phone|password|card_number|cvv|stripe_secret|api_key)'

PII_VIOLATIONS=$(
    grep -rn --include="*.go" \
        -E 'fmt\.(Print|Printf|Println|Fprint|Fprintf|Fprintln)\(' \
        "${SERVICES_DIR}" \
        | grep -v '_test\.go:' \
        | grep -iE "${PII_PATTERN}" \
        | grep -v '//nolint:logging-smoke' \
        | grep -v '// nolint:logging-smoke' \
        || true
)

if [[ -n "${PII_VIOLATIONS}" ]]; then
    echo "FAIL: fmt.Print* calls with PII-adjacent field names detected."
    echo "These values must go through a structured logger with field-level redaction."
    echo ""
    echo "${PII_VIOLATIONS}"
    echo ""
    FAILED=1
else
    echo "PASS: no PII-adjacent fields in fmt.Print* calls."
fi

# ---- Summary -----------------------------------------------------------------
echo ""
if [[ "${FAILED}" -eq 1 ]]; then
    echo "RESULT: FAIL — see violations above."
    exit 1
else
    echo "RESULT: PASS — log-shipping smoke guard clean."
    exit 0
fi
