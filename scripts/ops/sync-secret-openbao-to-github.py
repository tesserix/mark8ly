#!/usr/bin/env python3
"""Copy a scoped OpenBao application value to a GitHub Actions secret."""

import argparse
import re
import subprocess


def sync(path: str, github_secret: str, repo: str) -> None:
    if not re.fullmatch(r"mark8ly(?:-uat)?/app/mark8ly-[a-z0-9-]+", path):
        raise ValueError("Require an exact Mark8ly application path")
    if not re.fullmatch(r"[A-Z_][A-Z0-9_]*", github_secret):
        raise ValueError("Invalid GitHub secret name")
    if not re.fullmatch(r"tesserix/[a-z0-9-]+", repo):
        raise ValueError("Require an explicit Tesserix repository")
    result = subprocess.run(
        ["bao", "kv", "get", "-field=value", "kv/" + path],
        capture_output=True,
        check=False,
        timeout=30,
    )
    if result.returncode or not result.stdout:
        raise RuntimeError("OpenBao read failed or returned an empty value")
    result = subprocess.run(
        ["gh", "secret", "set", github_secret, "--repo", repo],
        input=result.stdout,
        capture_output=True,
        check=False,
        timeout=30,
    )
    if result.returncode:
        raise RuntimeError("GitHub secret update failed; diagnostics withheld")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("path")
    parser.add_argument("github_secret")
    parser.add_argument("--repo", default="tesserix/mark8ly")
    args = parser.parse_args()
    try:
        sync(args.path, args.github_secret, args.repo)
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError):
        parser.exit(
            1,
            "Copy failed; verify the path, scoped OpenBao session and GitHub access.\n",
        )
    print("GitHub secret updated from OpenBao; payload withheld.")


if __name__ == "__main__":
    main()
