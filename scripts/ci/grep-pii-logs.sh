#!/usr/bin/env bash
# =============================================================================
# PII-leak guard — grep added log lines for suspected PII fields.
# =============================================================================
# Runs over the diff between the PR base branch and HEAD. Only added lines
# (starting with '+') are inspected so the job doesn't re-flag pre-existing
# violations. A match exits 1; the `pii-audit-cleared` PR label skips the
# workflow entirely (see .github/workflows/pii-leak-guard.yml).
#
# Supported log call patterns:
#   slog.Debug|Info|Warn|Error            (log/slog)
#   logrus.Debug|Info|Warn|Error          (sirupsen/logrus)
#   Logger.Debug|Info|Warn|Error          (method call on a bound logger)
#   WithField(                            (logrus.WithField-style)
#
# Sensitive fields (case-insensitive):
#   email, phone, tax[_-]?id, card, cvv, address, passport, ssn, dob,
#   date[_-]?of[_-]?birth, national[_-]?id, pan[_-]?number, aadhaar,
#   password, token, refresh[_-]?token, access[_-]?token
# =============================================================================
set -euo pipefail

BASE_REF="${GITHUB_BASE_REF:-main}"

# Local / CI fallback — if origin/${BASE_REF} exists prefer it; otherwise
# fall back to reading from stdin (supports the sanity-check pattern
# `echo '+slog.Info(...)' | grep-pii-logs.sh`).
if [[ -t 0 ]]; then
    if git rev-parse --verify --quiet "origin/${BASE_REF}" >/dev/null; then
        BASE="origin/${BASE_REF}"
    elif git rev-parse --verify --quiet "${BASE_REF}" >/dev/null; then
        BASE="${BASE_REF}"
    else
        echo "ERROR: cannot resolve base ref '${BASE_REF}' (tried origin/${BASE_REF} and ${BASE_REF})." >&2
        exit 2
    fi
    DIFF=$(git diff --unified=0 "${BASE}"...HEAD -- 'services/**/*.go' ':!**/*_test.go')
else
    # stdin mode — useful for local sanity checks. Grep the incoming text
    # directly instead of calling git diff.
    DIFF=$(cat)
fi

if [[ -z "${DIFF}" ]]; then
    echo "PASS: no Go source changes to inspect."
    exit 0
fi

PII_FIELDS='(email|phone|tax[_-]?id|card|cvv|address|passport|ssn|dob|date[_-]?of[_-]?birth|national[_-]?id|pan[_-]?number|aadhaar|password|token|refresh[_-]?token|access[_-]?token)'
LOG_CALL='(slog\.(Debug|Info|Warn|Error)|logrus\.(Debug|Info|Warn|Error)|Logger\.(Debug|Info|Warn|Error)|WithField\s*\()'

# Only added lines. Drop hunk-header markers ('+++ b/...').
ADDED=$(printf '%s\n' "${DIFF}" | grep -E '^\+' | grep -Ev '^\+\+\+ ')

VIOLATIONS=$(printf '%s\n' "${ADDED}" | grep -E "${LOG_CALL}" | grep -iE "${PII_FIELDS}" || true)

if [[ -n "${VIOLATIONS}" ]]; then
    echo "FAIL: possible PII leak in log statements:"
    echo "${VIOLATIONS}"
    echo ""
    echo "False positive (hashed / redacted value)?"
    echo "Apply PR label 'pii-audit-cleared' after reviewer verification, then re-run CI."
    exit 1
fi

echo "PASS: no suspected PII in added log lines."
