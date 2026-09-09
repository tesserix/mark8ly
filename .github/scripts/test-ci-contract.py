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

        # NEXT_PUBLIC_GOOGLE_CLIENT_ID is deliberately NOT required any more
        # (#708). It existed so GIP's Google Identity Services script had a
        # browser client id to initialise with. Google sign-in is now a
        # full-page redirect through Zitadel's IDP intent — the client id
        # lives on the Zitadel IDP, server-side, and no browser bundle needs
        # it. Requiring it here would force every app to keep baking a value
        # nothing reads.
        next_apps = ["onboarding", "admin", "storefront"]
        for app in next_apps:
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
                # The GIP browser client id must NOT come back: it would be a
                # dead value baked into the bundle, and its presence would
                # imply a client-side Google flow that no longer exists.
                self.assertNotIn("NEXT_PUBLIC_GOOGLE_CLIENT_ID", contents)
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

    def test_every_node_gate_matches_the_shipped_node_major(self) -> None:
        """No gate may run a Node major that production does not ship (#857).

        CI ran every Node gate on 22 while the app Dockerfiles ship
        base-node-*-24, so no gate exercised the runtime that ships -- which
        had already left two Node-24-only test failures green in CI. The
        Dockerfile bases are the source of truth here: they are what runs in
        production, and they are digest-pinned and updated by the base image
        refresh workflow. Every other Node declaration must agree with them.
        """
        majors: dict[str, set[str]] = {}

        def record(label: str, value: str) -> None:
            majors.setdefault(value, set()).add(label)

        for app in ("admin", "storefront", "onboarding"):
            dockerfile = (ROOT / f"apps/{app}/Dockerfile").read_text()
            found = set(re.findall(r"base-node-(?:builder|runtime)-(\d+)@", dockerfile))
            self.assertNotEqual(
                found, set(), f"apps/{app}/Dockerfile pins no base-node-* image"
            )
            for major in found:
                record(f"apps/{app}/Dockerfile base-node-*-{major}", major)

        # `node_version:`/`node-version:` in any workflow, and the root
        # package.json engines floor. Globbing the workflow dir rather than
        # naming files means a NEW workflow with a Node pin is covered the
        # day it lands, which is how the drift got in the first time.
        workflow_dir = ROOT / ".github/workflows"
        for path in sorted(
            p
            for pattern in ("*.yml", "*.yaml")
            for p in workflow_dir.glob(pattern)
        ):
            for major in re.findall(
                r'^\s*node[_-]version:\s*"?(\d+)', path.read_text(), re.MULTILINE
            ):
                record(f"{path.name} node_version", major)

        engines = json.loads((ROOT / "package.json").read_text())["engines"]["node"]
        engines_major = re.search(r"(\d+)", engines)
        self.assertIsNotNone(engines_major, f"unparseable engines.node: {engines}")
        record(f"package.json engines.node ({engines})", engines_major.group(1))

        self.assertEqual(
            len(majors),
            1,
            "Node major drift -- every gate must run the major production "
            "ships:\n"
            + "\n".join(
                f"  {major}: {', '.join(sorted(labels))}"
                for major, labels in sorted(majors.items())
            ),
        )

    def test_no_workflow_sets_the_opt_in_operator_flags(self) -> None:
        # mark8ly#834 Task 1 moved 8 opt-in operator scripts to
        # apps/*/tests/operator/ — five of them default to a PRODUCTION
        # host and seed data / audit live systems. They are gated behind
        # env vars that must be set by hand on a developer's machine, never
        # by CI. A workflow that set one of these would run the FULL_FLOW
        # golden path, or a real audit or image-seeding pass, against
        # production on every push.
        #
        # Both extensions: GitHub accepts .yml and .yaml equally, so a
        # single-extension glob would let a future *.yaml workflow set one
        # of these flags and never be checked.
        workflow_dir = ROOT / ".github/workflows"
        workflows = sorted(
            path
            for pattern in ("*.yml", "*.yaml")
            for path in workflow_dir.glob(pattern)
        )
        offenders = []
        for path in workflows:
            contents = path.read_text()
            for flag in ("FULL_FLOW", "ADMIN_AUDIT", "STOREFRONT_AUDIT", "SEED_IMAGES"):
                if flag in contents:
                    offenders.append(f"{path.name} sets/references {flag}")
        self.assertEqual(
            offenders,
            [],
            "a workflow references an opt-in operator flag — these must "
            "only ever be set by hand, never by CI:\n" + "\n".join(offenders),
        )

    def test_no_install_path_relaxes_peer_resolution(self) -> None:
        """No CI or image install may use legacy peer resolution (#869).

        `--legacy-peer-deps` was in every install path for exactly one
        reason: apps/mobile-admin mixed expo@56 with expo-router@57, which a
        plain `npm install` cannot resolve. #869 realigned those pins, so the
        flag no longer has a job.

        Keeping it out matters more than removing it did. Under the flag a
        package can import something it never declared and survive on
        hoisting until a version moves -- that shipped twice (#863's five
        undeclared expo peers, #867's 37 @tiptap/core call sites) and left
        `expo-web-browser` and `react-native-otp-entry` unresolvable in
        apps/mobile-admin's own typecheck. It also makes `npm install
        --legacy-peer-deps` -- the obvious move when an advisory lands --
        destructive: it prunes every peer-only entry from the lockfile.

        Without the flag a cross-major bump fails loudly at install instead.

        Globbing both workflow extensions and every Dockerfile means a NEW
        file reintroducing the flag is covered the day it lands, which is
        how it spread in the first place.
        """
        offenders = []

        workflow_dir = ROOT / ".github/workflows"
        for path in sorted(
            p
            for pattern in ("*.yml", "*.yaml")
            for p in workflow_dir.glob(pattern)
        ):
            for lineno, line in enumerate(path.read_text().splitlines(), 1):
                if line.lstrip().startswith("#"):
                    continue  # prose about the flag is fine; using it is not
                if "--legacy-peer-deps" in line or "legacy_peer_dependencies" in line:
                    offenders.append(f".github/workflows/{path.name}:{lineno}")

        for dockerfile in sorted(ROOT.glob("apps/*/Dockerfile")):
            for lineno, line in enumerate(dockerfile.read_text().splitlines(), 1):
                if line.lstrip().startswith("#"):
                    continue
                if "--legacy-peer-deps" in line:
                    rel = dockerfile.relative_to(ROOT)
                    offenders.append(f"{rel}:{lineno}")

        self.assertEqual(
            offenders,
            [],
            "legacy peer resolution is back in an install path -- it hides "
            "undeclared imports and makes `npm install` prune peer deps "
            "(#869):\n" + "\n".join(offenders),
        )

    def test_mobile_admin_expo_packages_share_one_sdk_major(self) -> None:
        """Every expo-versioned pin in mobile-admin must match `expo` (#869).

        Expo publishes expo-* and jest-expo on the SDK's own major, so
        `expo@~56` pairs with `expo-router@~56`, `expo-constants@~56` and so
        on. Dependabot does not know that: #84, #86 and #88 each bumped one
        package across a major on its own, which is what produced the
        unresolvable tree.

        The install now fails on such a bump too, but with an ERESOLVE dump
        that names transitive peers rather than the mistake. This fails at
        the policy gate instead, and says which package drifted.

        Scoped to mobile-admin deliberately: apps/mobile-storefront is on
        SDK 52, which predates unified SDK versioning (expo-constants@~17,
        expo-device@~7), so the rule does not hold there.

        @expo-google-fonts/* is excluded -- it versions independently of the
        SDK (currently ^0.4.0).
        """
        manifest = json.loads((ROOT / "apps/mobile-admin/package.json").read_text())
        pins = {**manifest["dependencies"], **manifest["devDependencies"]}

        def major(spec: str) -> str:
            found = re.search(r"(\d+)", spec)
            self.assertIsNotNone(found, f"unparseable version range: {spec}")
            return found.group(1)

        sdk = major(pins["expo"])
        self.assertGreaterEqual(
            int(sdk), 56, "this rule assumes unified SDK versioning (SDK >= 53)"
        )

        drifted = {
            name: spec
            for name, spec in sorted(pins.items())
            if (name == "expo" or name.startswith("expo-") or name == "jest-expo")
            and major(spec) != sdk
        }
        self.assertEqual(
            drifted,
            {},
            f"apps/mobile-admin is on Expo SDK {sdk}; these pins are on "
            "another major, which npm cannot resolve (#869):\n"
            + "\n".join(f"  {n}: {v}" for n, v in drifted.items()),
        )

    def test_e2e_workflows_pr_and_push_watch_the_same_paths(self) -> None:
        """Every e2e workflow must filter both triggers on one path list (#858).

        These are expensive (a docker compose stack plus cold Next builds), so
        they run only when a PR touches what they test. That is safe only
        while `pull_request` and `push` agree: a path listed under `push`
        alone is a path whose breakage is found after the merge instead of on
        the PR that caused it.

        Each must also watch ITSELF, which is the property that was missing
        when e2e-onboarding.yml reached review having never once executed --
        a PR editing the workflow is then a PR that runs it.

        Globbed rather than named: e2e-admin.yml was added later and would
        not have been covered by a test that named only the onboarding one.
        That is the same mistake the build-context guard made before it was
        derived rather than restated.

        Parsed rather than grepped: PyYAML resolves the bare `on:` key to the
        boolean True, which is why this reads d[True] with a fallback.
        """
        try:
            import yaml
        except ImportError:  # pragma: no cover - yaml ships on the runner
            self.skipTest("PyYAML unavailable")

        workflow_dir = ROOT / ".github/workflows"
        e2e = sorted(workflow_dir.glob("e2e-*.yml")) + sorted(
            workflow_dir.glob("e2e-*.yaml")
        )
        # A rename that dodged the glob would make this vacuously pass.
        self.assertGreaterEqual(
            len(e2e), 2, "expected the onboarding and admin e2e workflows"
        )

        for path in e2e:
            workflow = yaml.safe_load(path.read_text())
            triggers = workflow[True] if True in workflow else workflow["on"]

            # e2e-runnable.yml has no paths filter -- it is the cheap canary
            # and runs on every PR deliberately.
            if "paths" not in triggers.get("push", {}):
                continue

            self.assertIn(
                "pull_request",
                triggers,
                f"{path.name} lost its pull_request trigger -- without it the "
                "workflow can reach main having never executed (#858)",
            )
            self.assertEqual(
                triggers["pull_request"]["paths"],
                triggers["push"]["paths"],
                f"{path.name}'s pull_request and push path filters have "
                "diverged; a path watched only on push is found only after merge",
            )
            self.assertIn(
                f".github/workflows/{path.name}",
                triggers["pull_request"]["paths"],
                f"{path.name} must watch itself, so a PR editing it runs it",
            )


if __name__ == "__main__":
    unittest.main()
