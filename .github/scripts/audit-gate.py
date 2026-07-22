#!/usr/bin/env python3
"""npm-audit gate with a targeted advisory allowlist.

Reads `npm audit --json` output and fails (exit 1) on any high/critical
vulnerability whose advisory set is NOT fully covered by ALLOWLIST.

Why an allowlist exists: a 2026 CVE batch flagged sharp<0.35.0 (libvips) —
pulled transitively by next, which optionally pins sharp ^0.34.5, so a clean
bump needs a next upgrade (tracked follow-up). Every OTHER high/critical still
fails the gate, and if next later picks up an UNRELATED advisory, its advisory
set stops being a subset of ALLOWLIST and the gate fails again — so this does
not blanket-exempt the package.
"""
import json
import sys

# GHSA ids intentionally tolerated. Keep this list SHORT and time-boxed.
ALLOWLIST = {
    "GHSA-f88m-g3jw-g9cj",  # sharp<0.35 libvips CVEs (via next); fix: bump next
}


def advisory_ids(name, vulns, seen=None):
    """All GHSA/CVE ids a vuln stems from, resolving string `via` references."""
    seen = seen or set()
    if name in seen:
        return set()
    seen.add(name)
    v = vulns.get(name)
    if not v:
        return set()
    ids = set()
    for via in v.get("via", []):
        if isinstance(via, str):
            ids |= advisory_ids(via, vulns, seen)
        elif isinstance(via, dict):
            url = via.get("url", "")
            gh = url.rstrip("/").split("/")[-1] if url else ""
            if gh:
                ids.add(gh)
    return ids


def main():
    data = json.load(open(sys.argv[1]))
    vulns = data.get("vulnerabilities", {})

    blocking = []
    for name, v in vulns.items():
        if v.get("severity") not in ("high", "critical"):
            continue
        ids = advisory_ids(name, vulns)
        # Non-empty AND fully inside the allowlist → tolerate. An empty id set
        # means we couldn't attribute it — fail closed rather than pass it.
        if ids and ids <= ALLOWLIST:
            print(f"  allowlisted: {name} ({v.get('severity')}) -> {sorted(ids)}")
            continue
        blocking.append((v.get("severity"), name, sorted(ids)))

    if blocking:
        print("BLOCKING high/critical vulnerabilities:")
        for sev, name, ids in blocking:
            print(f"  {sev}: {name} {ids}")
        sys.exit(1)
    print("npm audit gate passed (allowlisted advisories excluded).")


if __name__ == "__main__":
    main()
