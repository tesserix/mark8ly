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
  const cfg = readFileSync(
    join(root, "apps/onboarding/playwright.config.ts"),
    "utf8",
  );
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
  expect(
    overrideVarLine,
    "Makefile has no COMPOSE_OVERRIDE variable",
  ).toBeTruthy();
  expect(overrideVarLine).toMatch(/\$\(wildcard/);

  const composeLine = lines.find((l) => /^COMPOSE\s*:?=/.test(l));
  expect(composeLine, "Makefile has no COMPOSE variable").toBeTruthy();
  expect(composeLine).toMatch(/\$\(COMPOSE_OVERRIDE\)/);
});

// A migrate stage's CMD has no ENTRYPOINT, so `command: ["up"]` replaces the
// binary instead of being passed to it — the container execs "up" and fails
// immediately. Every migrate command must name its binary as argv[0] (e.g.
// ["/migrate", "up"]), never a bare ["up"].
test('no docker-compose service uses a bare command: ["up"]', () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  const commandLines = yml.match(/^\s*command:\s*\[[^\]]*\]/gm) ?? [];
  expect(commandLines.length).toBeGreaterThan(0);
  for (const line of commandLines) {
    expect(line).not.toMatch(/command:\s*\[\s*"up"\s*\]/);
  }
});

// platform-api must reach marketplace-api by its compose name (#858).
//
// Its config default is the production k8s DNS name. Unset here, every
// onboarding Complete fails DNS three times with backoff -- ensure-self-vendor,
// ensure-self-store, ensure-subscription -- taking ~20s and logging
// "THIS STORE HAS NO TRIAL CLOCK". The store is still created and the request
// still returns 200, so nothing fails: the dev stack silently produced
// trial-less stores, and the e2e specs merely timed out at 15s looking like a
// backend problem.
test("platform-api reaches marketplace-api by its compose service name", () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  const block = yml.match(/^ {2}platform-api:\n((?: {4}.*\n|\n)*)/m)?.[1];
  expect(block, "platform-api service block not found").toBeTruthy();
  const url = block?.match(/^ {6}MARKETPLACE_API_URL:\s*(\S+)/m)?.[1];
  expect(
    url,
    "platform-api has no MARKETPLACE_API_URL, so it keeps the k8s production " +
      "default and every onboarding Complete is slow and trial-less",
  ).toBeTruthy();
  expect(url).toMatch(/^http:\/\/marketplace-api:/);
});

// Zitadel mints its bootstrap PAT only when an expiry is declared (#858).
//
// With the machine-user keys but no PAT block, v4.15.3 starts cleanly, serves
// healthz, and writes NOTHING to PATPATH -- so the bootstrap fails later with
// an empty token file and the cause is three steps upstream.
test("zitadel declares a first-instance PAT expiry, or no token is minted", () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  const block = yml.match(/^ {2}zitadel:\n((?: {4}.*\n|\n)*)/m)?.[1];
  expect(block, "zitadel service block not found").toBeTruthy();
  expect(block).toMatch(/ZITADEL_FIRSTINSTANCE_PATPATH:/);
  expect(
    block,
    "ZITADEL_FIRSTINSTANCE_ORG_MACHINE_PAT_EXPIRATIONDATE is required: " +
      "without it the machine user exists but no PAT is ever written",
  ).toMatch(/ZITADEL_FIRSTINSTANCE_ORG_MACHINE_PAT_EXPIRATIONDATE:/);
});

// A distroless runtime cannot run a shell-based healthcheck (#858).
//
// marketplace-api declared `test: ["CMD", "wget", ...]` while its runtime
// stage is base-distroless-static, which ships no shell, wget or curl. The
// probe could never pass, so the container sat permanently `unhealthy` while
// serving 200s -- and nothing noticed, because the service could not start
// at all until #875. The real cost is downstream: a
// `condition: service_healthy` on such a service blocks forever.
//
// The compose file already records this for openfga in prose. This makes it
// a rule, derived from each service's actual runtime base.
test("no service with a distroless runtime declares a shell healthcheck", () => {
  const yml = readFileSync(join(root, "infra/dev/docker-compose.yml"), "utf8");
  const offenders: string[] = [];

  for (const m of yml.matchAll(
    /^ {2}([a-z0-9-]+):\n((?: {4}\S.*\n| {4,}.*\n|\n)*)/gm,
  )) {
    const service = m[1];
    const body = m[2];
    if (!service || !body) continue;
    if (!/^ {4}healthcheck:/m.test(body)) continue;

    const probe = body.match(/^ {6}test:\s*(.+)$/m)?.[1] ?? "";
    if (!/wget|curl|CMD-SHELL/.test(probe)) continue;

    // Which Dockerfile backs this service, if any? Services built from a
    // published image (postgres:15-alpine) are fine -- they have a shell.
    const context = body.match(/^ {6}context:\s*(\S+)/m)?.[1];
    if (!context) continue;
    // `dockerfile:` is relative to the CONTEXT, not to the compose file.
    // Resolving it against infra/dev instead made every read throw, and the
    // catch below swallowed it -- the first version of this test passed
    // against the very defect it was written for.
    const dockerfileRel =
      body.match(/^ {6}dockerfile:\s*(\S+)/m)?.[1] ?? "Dockerfile";

    let dockerfile: string;
    try {
      dockerfile = readFileSync(
        join(root, "infra/dev", context, dockerfileRel),
        "utf8",
      );
    } catch {
      continue; // the build-context test above owns unreadable Dockerfiles
    }

    // The stage the service runs: `target:`, else the final FROM.
    const target = body.match(/^ {6}target:\s*(\S+)/m)?.[1];
    const stages = [
      ...dockerfile.matchAll(/^FROM\s+(\S+)(?:\s+AS\s+(\S+))?/gm),
    ];
    const stage = target
      ? stages.find((f) => f[2] === target)
      : stages[stages.length - 1];
    if (!stage) continue;

    if (/distroless/.test(stage[1] ?? "")) {
      offenders.push(
        `${service}: healthcheck runs ${probe.trim()} but its runtime stage is ${stage[1]} (no shell/wget/curl)`,
      );
    }
  }

  expect(offenders.join("\n")).toBe("");
});

// Every compose build context must be able to see what its Dockerfile
// COPYs (#858).
//
// The stage-1 version of this test named platform-api explicitly, so when
// marketplace-api carried the identical defect -- a service-local context
// under a Dockerfile that COPYs services/ AND packages/ -- it passed. The
// service that was measured got fixed and the one that was not stayed
// broken, and `marketplace-api` had therefore never once started in the
// dev stack.
//
// So this derives the rule instead of restating one service's answer: read
// each service's real COPY sources and check they are reachable from the
// declared context. A new service is covered the day it lands.
test("every compose build context can see what its Dockerfile COPYs", () => {
  const composePath = join(root, "infra/dev/docker-compose.yml");
  const yml = readFileSync(composePath, "utf8");

  // service name -> { context, dockerfile }. Compose resolves both relative
  // to the compose file's own directory, infra/dev.
  const builds = new Map<string, { context: string; dockerfile: string }>();
  for (const m of yml.matchAll(
    /^ {2}([a-z0-9-]+):\n {4}build:\n((?: {6}[a-z_]+:.*\n)+)/gm,
  )) {
    const service = m[1];
    const block = m[2];
    if (!service || !block) continue;
    const context = block.match(/^ {6}context:\s*(\S+)/m)?.[1];
    if (!context) continue;
    const dockerfile =
      block.match(/^ {6}dockerfile:\s*(\S+)/m)?.[1] ?? "Dockerfile";
    builds.set(service, { context, dockerfile });
  }
  // A compose file whose build blocks stopped parsing would vacuously pass.
  expect(builds.size).toBeGreaterThan(0);

  const offenders: string[] = [];
  for (const [service, { context, dockerfile }] of builds) {
    const contextDir = join(root, "infra/dev", context);
    const dockerfilePath = join(contextDir, dockerfile);

    let contents: string;
    try {
      contents = readFileSync(dockerfilePath, "utf8");
    } catch {
      offenders.push(
        `${service}: dockerfile ${dockerfile} does not exist under context ${context}`,
      );
      continue;
    }

    // COPY sources, skipping --from=<stage> (those read another build
    // stage, never the context) and --chown-style flags.
    for (const line of contents.split("\n")) {
      if (!/^COPY\s/.test(line) || /--from=/.test(line)) continue;
      const args = line
        .replace(/^COPY\s+/, "")
        .split(/\s+/)
        .filter((a) => !a.startsWith("--"));
      for (const src of args.slice(0, -1)) {
        if (src.startsWith("/")) continue;
        try {
          readFileSync(join(contextDir, src));
        } catch (err) {
          const code = (err as NodeJS.ErrnoException).code;
          // EISDIR means the path resolved to a directory -- reachable,
          // which is all this asserts. ENOENT means it is not in context.
          if (code === "ENOENT") {
            offenders.push(
              `${service}: ${dockerfile} COPYs "${src}", unreachable from context "${context}"`,
            );
          }
        }
      }
    }
  }

  expect(offenders.join("\n")).toBe("");
});
