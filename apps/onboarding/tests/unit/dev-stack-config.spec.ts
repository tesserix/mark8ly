import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

const root = join(__dirname, "../../../..");

// GIP was removed on 2026-09-08 (#GIP removal, 21/21). A secret loader that
// still pulls GIP_* values makes `make dev` depend on GCP for credentials
// nothing reads, which is why the local stack was unusable.
test("the dev secret loader no longer pulls GIP values", () => {
  const sh = readFileSync(join(root, "infra/dev/load-secrets.sh"), "utf8");
  expect(sh).not.toMatch(/GIP_/);
  expect(sh).not.toMatch(/NEXT_PUBLIC_GIP_/);
});

// The compose header told developers auth-bff talks to Google Identity
// Platform. It does not, and a stale orientation comment is worse than none.
test("the compose header does not claim a GIP dependency", () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  expect(yml).not.toMatch(/Google Identity Platform/);
});

// The onboarding Playwright config still lists a "firebase auth emulator"
// among the services it assumes are up. There has never been one since GIP
// was removed, and it sends anyone debugging a failure looking for a
// container that does not exist. NOTE: do not assert this against
// docker-compose.yml — it contains zero occurrences of "firebase", so the
// assertion would pass vacuously and guard nothing.
test("the onboarding playwright config does not reference a firebase emulator", () => {
  const cfg = readFileSync(join(root, "apps/onboarding/playwright.config.ts"), "utf8");
  expect(cfg).not.toMatch(/firebase/i);
});

// The Tier-B subset must come up with no GCP access, so the target that
// brings it up must not depend on the secret-pulling target.
test("dev-min does not depend on dev-secrets", () => {
  const mk = readFileSync(join(root, "Makefile"), "utf8");
  const line = mk.split("\n").find((l) => l.startsWith("dev-min:"));
  expect(line, "Makefile has no dev-min target").toBeTruthy();
  expect(line).not.toMatch(/dev-secrets/);
});

// Compose only auto-merges docker-compose.override.yml when no -f is given.
// The Makefile passes -f explicitly, so an override that exists (e.g. to
// drop postgres's host port on a machine where 5432 is already taken) is
// silently inert unless the Makefile also references it (#858).
//
// Two things must both hold, or a partial revert slips through undetected:
// COMPOSE_OVERRIDE must actually be defined from $(wildcard ...), and the
// COMPOSE variable itself (not just some other line, and not just the file
// as a whole) must reference it. A naive `mk.split("\n").find(l =>
// l.startsWith("COMPOSE"))` matches "COMPOSE_OVERRIDE := ..." first, and a
// bare `expect(mk).toMatch(...)` over the whole file is satisfied by the
// explanatory comment alone — neither would catch someone reverting just
// the `-f $(COMPOSE_OVERRIDE)` clause from the COMPOSE line while leaving
// the comment and COMPOSE_OVERRIDE variable in place.
test("the Makefile's COMPOSE variable references docker-compose.override.yml", () => {
  const mk = readFileSync(join(root, "Makefile"), "utf8");
  const lines = mk.split("\n");

  const overrideVarLine = lines.find((l) => /^COMPOSE_OVERRIDE\s*:?=/.test(l));
  expect(overrideVarLine, "Makefile has no COMPOSE_OVERRIDE variable").toBeTruthy();
  expect(overrideVarLine).toMatch(/\$\(wildcard/);

  const composeLine = lines.find((l) => /^COMPOSE\s*:?=/.test(l));
  expect(composeLine, "Makefile has no COMPOSE variable").toBeTruthy();
  expect(composeLine).toMatch(/\$\(COMPOSE_OVERRIDE\)/);
});
