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
    # brace-expansion DoS — reaches us ONLY via the eslint dev-toolchain
    # (eslint-config-next -> eslint -> minimatch), a devDependency NOT present
    # in the runtime Docker images, so no production exposure. Installed tree is
    # already on the patched 1.1.16/2.1.2/5.0.7 lines; this 2026 advisory flags
    # them anyway. Proper bump blocked by the monorepo multi-version lockfile
    # (can't regen locally). Time-boxed: drop when eslint deps are refreshed.
    "GHSA-mh99-v99m-4gvg",
    # image-size DoS via infinite loops in the ICNS (GHSA-w3rx-r6r6-pgpr) and
    # JXL/HEIF (GHSA-5p2g-fcmc-qvqq) parsers. Reaches us only as a BUILD-TIME
    # dependency of the React Native bundler:
    #   apps/mobile-* -> react-native -> @react-native/community-cli-plugin
    #                 -> metro -> image-size
    # metro uses it to measure image assets while bundling, on a developer's
    # machine or in the mobile build job. It is not present in any deployed web
    # service image, and nothing in production feeds untrusted ICNS/JXL/HEIF
    # through it, so there is no production exposure to a parser DoS here.
    #
    # NOT time-boxed to an upgrade, because there is nothing to upgrade to:
    # both advisories list `image-size <= 2.0.2` affected with first patched
    # version NONE, and 2.0.2 is the latest published release. An `overrides`
    # pin therefore cannot fix it either — clearing this needs image-size to
    # ship a fix, then metro, then react-native.
    #
    # Left un-allowlisted, these block EVERY pr in the repo: main went red on a
    # completely unchanged lockfile when npm's audit feed picked the advisories
    # up (they were published 2026-06-10; the gate started failing 2026-09-02).
    # Revisit when react-native is next bumped — drop these two ids, re-run the
    # gate, and see whether the chain has been patched upstream.
    "GHSA-w3rx-r6r6-pgpr",
    "GHSA-5p2g-fcmc-qvqq",
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
