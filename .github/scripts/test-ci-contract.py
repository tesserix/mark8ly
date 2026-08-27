#!/usr/bin/env python3

import json
import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/ci.yml"
IMAGES = ROOT / ".github/ci/container-images.json"
CANDIDATE_REF = "29f963da2412a4ba0c755f19697ad0a31d7624b4"
RELEASE_REF = "v2.2.1"


class ReusableCIContract(unittest.TestCase):
    def test_caller_is_thin_explicit_and_fail_closed(self) -> None:
        workflow = WORKFLOW.read_text()
        meaningful = [
            line
            for line in workflow.splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        ]

        self.assertLessEqual(len(meaningful), 180)
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
                self.assertIn("id=PACKAGE_READ_TOKEN", contents)
                self.assertIn("id=APPLICATION_BUILD_SECRET", contents)
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

    def test_dependabot_tracks_every_pinned_dockerfile(self) -> None:
        config = (ROOT / ".github/dependabot.yml").read_text()
        for directory in (
            "/services/platform-api",
            "/services/auth-bff",
            "/services/marketplace-api",
            "/services/otto",
            "/apps/onboarding",
            "/apps/admin",
            "/apps/storefront",
        ):
            self.assertRegex(
                config,
                rf"package-ecosystem:\s*docker\s+directory:\s*{re.escape(directory)}\b",
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
