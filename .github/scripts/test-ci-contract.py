#!/usr/bin/env python3

import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/ci.yml"
IMAGES = ROOT / ".github/ci/container-images.json"
CANDIDATE_REF = "29f963da2412a4ba0c755f19697ad0a31d7624b4"
RELEASE_REF = "v2.3.0"


class ReusableCIContract(unittest.TestCase):
    def test_caller_is_thin_explicit_and_fail_closed(self) -> None:
        workflow = WORKFLOW.read_text()
        meaningful = [
            line
            for line in workflow.splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        ]

        # The caller stays thin: product policy only, everything else lives in
        # tesserix-workflows. Raised 180 -> 181 for the fifth Go module
        # (packages/platformauth, #720). Bump deliberately, one module at a
        # time — the cap is meant to make growth argue for itself.
        #
        # Do NOT buy headroom here by writing a matrix entry as a single-line
        # flow mapping: prettier expands it to five lines (`npm run
        # format:check` covers .github/**/*.yml) and the caller fails CI for
        # formatting instead.
        self.assertLessEqual(len(meaningful), 181)
        self.assertNotIn("secrets: inherit", workflow)
        self.assertNotIn("continue-on-error", workflow)
        self.assertFalse(
            (ROOT / ".github/workflows/reusable-security.yml").exists()
        )
        self.assertIn("name: CI gate", workflow)
        self.assertIn("if: ${{ always() }}", workflow)
        self.assertIn("HAS_FAILURE", workflow)
        self.assertIn("HAS_CANCELLED", workflow)
        for workspace in (
            "@mark8ly/admin",
            "@mark8ly/storefront",
            "@mark8ly/onboarding",
            "@repo/ui",
            "@repo/otto-widget",
        ):
            self.assertIn(workspace, workflow)

        called = re.findall(
            r"tesserix/tesserix-workflows/\.github/workflows/([^@]+)@([^\s]+)",
            workflow,
        )
        expected = {
            "go-ci.yml",
            "nextjs-ci.yml",
            "secret-scan.yml",
            "container-ci.yml",
            "container-release.yml",
        }
        self.assertEqual(expected, {name for name, _ in called})
        self.assertTrue(called)
        refs = {ref for _, ref in called}
        self.assertEqual(1, len(refs))
        self.assertIn(refs.pop(), {CANDIDATE_REF, RELEASE_REF})

    def test_language_matrix_owns_measured_coverage_floors(self) -> None:
        workflow = WORKFLOW.read_text()
        for service, coverage in {
            "platform-api": 25,
            "auth-bff": 31,
            "marketplace-api": 21,
            "otto": 4,
        }.items():
            self.assertRegex(
                workflow,
                rf"service:\s*{service}\s+directory:\s*services/{service}\s+coverage:\s*{coverage}\b",
            )

    def test_image_matrix_covers_every_deployable(self) -> None:
        images = json.loads(IMAGES.read_text())["include"]
        by_name = {image["name"]: image for image in images}

        self.assertEqual(
            {
                "mark8ly-platform-api",
                "mark8ly-auth-bff",
                "mark8ly-marketplace-api",
                "mark8ly-otto",
                    "mark8ly-onboarding",
                "mark8ly-admin",
                "mark8ly-storefront",
            },
            set(by_name),
        )
        self.assertEqual(len(images), len(by_name))

        expected = {
            "mark8ly-platform-api": ("server", 0, "/health"),
            "mark8ly-auth-bff": ("server", 0, "/health"),
            "mark8ly-marketplace-api": ("runtime", 0, "/health"),
            "mark8ly-otto": ("server", 0, "/health"),
            "mark8ly-onboarding": ("runtime", 4201, "/api/health"),
            "mark8ly-admin": ("runtime", 4202, "/api/health"),
            "mark8ly-storefront": ("runtime", 4203, "/api/health"),
        }
        required = {
            "name",
            "context",
            "dockerfile",
            "target",
            "source_root",
            "build_args",
            "requires_application_build_secret",
            "trivy_ignore_file",
            "smoke_port",
            "smoke_path",
        }
        for name, (target, port, path) in expected.items():
            with self.subTest(name=name):
                image = by_name[name]
                self.assertEqual(required, set(image))
                self.assertEqual(target, image["target"])
                self.assertEqual(port, image["smoke_port"])
                self.assertEqual(path, image["smoke_path"])
                self.assertEqual(".trivyignore", image["trivy_ignore_file"])

        for name in ("mark8ly-onboarding", "mark8ly-admin", "mark8ly-storefront"):
            self.assertTrue(by_name[name]["requires_application_build_secret"])

    def test_dockerfiles_use_pinned_bases_and_generic_build_contract(self) -> None:
        dockerfiles = list((ROOT / "services").glob("*/Dockerfile")) + list(
            (ROOT / "apps").glob("*/Dockerfile")
        )
        selected = [
            path
            for path in dockerfiles
            if path.parent.name
            in {
                "platform-api",
                "auth-bff",
                "marketplace-api",
                "otto",
                "onboarding",
                "admin",
                "storefront",
            }
        ]
        self.assertEqual(7, len(selected))
        for path in selected:
            contents = path.read_text()
            with self.subTest(path=path.relative_to(ROOT)):
                self.assertNotRegex(
                    contents, re.compile(r"^FROM\s+\S+:latest", re.MULTILINE)
                )
                for line in contents.splitlines():
                    if line.startswith("FROM ghcr.io/tesserix/base-"):
                        self.assertRegex(line, r"@sha256:[0-9a-f]{64}\b")

        next_apps = {
            "onboarding": "REUSABLE_PUBLIC_BUILD_ARG_4",
            "admin": "REUSABLE_PUBLIC_BUILD_ARG_4",
            "storefront": "REUSABLE_PUBLIC_BUILD_ARG_5",
        }
        for app, google_slot in next_apps.items():
            contents = (ROOT / f"apps/{app}/Dockerfile").read_text()
            with self.subTest(app=app):
                # PACKAGE_READ_TOKEN is deliberately NOT required. Every
                # @tesserix/* package now comes from the public npm registry,
                # so `npm ci` needs no credential and the scoped-registry
                # .npmrc is gone. The reusable workflow still passes the
                # secret; nothing consumes it. Asserting its ABSENCE instead
                # would be wrong too — a Dockerfile is free to mount secrets
                # this contract does not know about.
                self.assertIn("id=APPLICATION_BUILD_SECRET", contents)
                # The registry move is only complete if no .npmrc is COPYed
                # in: the file no longer exists, so that instruction would
                # fail the build outright. Checked against COPY lines rather
                # than the whole file, so a comment explaining the migration
                # does not trip it.
                copied = [
                    line for line in contents.splitlines()
                    if line.startswith("COPY") and ".npmrc" in line
                ]
                self.assertEqual(copied, [], "Dockerfile still COPYs a deleted .npmrc")
                # The builder must carry the per-workspace node_modules, not
                # just the root tree. npm does not always hoist a workspace
                # dependency, and copying only /src/node_modules drops it —
                # which fails as "Module not found" during next build, long
                # after the install step reported success.
                self.assertIn("COPY --from=deps /src/apps ./apps", contents)
                self.assertIn("ARG REUSABLE_BUILD_CACHE_FP", contents)
                self.assertIn(
                    f"NEXT_PUBLIC_GOOGLE_CLIENT_ID=${google_slot}", contents
                )
                self.assertNotIn("id=NODE_AUTH_TOKEN", contents)
                self.assertNotIn(
                    "id=NEXT_SERVER_ACTIONS_ENCRYPTION_KEY", contents
                )
                self.assertNotIn("ARG SERVER_ACTIONS_KEY_FP", contents)
                self.assertNotIn("ARG REUSABLE_BUILD_SECRET_FP", contents)

    def test_base_image_pins_are_tracked_by_a_dependency_updater(self) -> None:
        # WHAT THIS PROTECTS, which is unchanged: every Dockerfile pinning a
        # company-owned ghcr.io/tesserix/base-* image must be tracked by
        # something that refreshes that pin. The pin is the exact thing that
        # goes stale when GHCR prunes old digests, and /services/mcp once went
        # untracked and rotted for precisely this reason.
        #
        # HOW it is tracked changed in #689. Dependabot listed each directory
        # explicitly in .github/dependabot.yml, so a new Dockerfile could be
        # forgotten and this test enumerated them to catch that. Renovate
        # DISCOVERS Dockerfiles itself, so there is no per-directory list to
        # fall out of step with — a strictly stronger guarantee, and the reason
        # the old assertion is gone rather than ported.
        #
        # WHAT THIS TEST CAN NO LONGER SEE, stated because the guarantee now
        # lives somewhere this repository cannot read: whether the docker
        # manager is enabled at all is decided by the shared preset
        # (github>tesserix/renovate-config). If someone sets `enabledManagers`
        # there without `dockerfile`, every base pin in this repo stops being
        # refreshed and NOTHING HERE FAILS. That check belongs in the preset's
        # own repository; this one asserts only that we are on renovate and
        # have not silently reverted to a config that tracks nothing.
        dockerfiles = [
            path
            for path in ROOT.rglob("Dockerfile")
            if "node_modules" not in path.parts and "vendor" not in path.parts
        ]
        self.assertTrue(dockerfiles, "no Dockerfiles found — discovery is broken")

        pinned = [
            path
            for path in dockerfiles
            if re.search(r"^FROM\s+ghcr\.io/tesserix/base-", path.read_text(), re.MULTILINE)
        ]
        self.assertTrue(
            pinned,
            "no Dockerfile pins a ghcr.io/tesserix/base- image — either the "
            "pins were removed or this discovery is broken; both want a human.",
        )

        renovate = ROOT / "renovate.json"
        self.assertTrue(
            renovate.is_file(),
            f"{len(pinned)} Dockerfiles pin a ghcr.io/tesserix/base- image but "
            "renovate.json is missing — nothing is refreshing those pins.",
        )

        config = json.loads(renovate.read_text())
        self.assertIn(
            "github>tesserix/renovate-config",
            config.get("extends", []),
            "renovate.json does not extend the shared org preset, so this "
            "repository's update policy is whatever happens to be in this file "
            "rather than the estate's.",
        )

        # The file dependabot read must be GONE, not merely unused. Leaving it
        # behind would give a reader two configs and no way to tell which one
        # is live — and this test would keep passing either way.
        self.assertFalse(
            (ROOT / ".github/dependabot.yml").exists(),
            "both .github/dependabot.yml and renovate.json exist; delete the "
            "dependabot config so there is one answer to what updates deps.",
        )

    def test_secret_baseline_is_redacted_and_fingerprint_specific(self) -> None:
        findings = json.loads((ROOT / ".gitleaks-baseline.json").read_text())
        self.assertTrue(findings)
        fingerprints = [finding["Fingerprint"] for finding in findings]
        self.assertEqual(len(fingerprints), len(set(fingerprints)))
        for finding in findings:
            self.assertIn("REDACTED", finding["Secret"])
            self.assertIn("REDACTED", finding["Match"])


if __name__ == "__main__":
    unittest.main()
