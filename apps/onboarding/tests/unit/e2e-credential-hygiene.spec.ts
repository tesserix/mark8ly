import { expect, test } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";

/**
 * Repo-wide guard: no e2e spec may carry a working credential, and no
 * captured browser session may be committed.
 *
 * `apps/admin/tests/e2e/admin-setup.spec.ts` defaulted every credential it
 * needed to a real value — admin email and password, Razorpay key pair,
 * Delhivery API key — with `ADMIN_BASE_URL` defaulting to the **production**
 * host. It sat in a PUBLIC repository for five months. Separately,
 * `apps/admin/tests/e2e/.state/` held committed Playwright `storageState`
 * captured from a real production run, including `httpOnly` session cookies
 * for `.mark8ly.com`.
 *
 * Neither was caught, and the near-miss is instructive: `.gitignore` already
 * ignored the `.audit/` sibling of that directory, the twin convention, and simply never
 * gained the `.state/` line. One directory was ignored, its sibling was
 * committed, and no gate distinguished them.
 *
 * These assertions are deliberately shape-based rather than value-based. A
 * guard that pinned the specific leaked strings would have to contain them,
 * which is the problem it is meant to solve.
 *
 * Scope note: this file lives under `apps/onboarding` because that is where
 * repo-wide guards already run in CI (see pricing-surfaces-truth.spec.ts),
 * not because the subject is onboarding-specific. It reads every app.
 */
const REPO_ROOT = path.join(__dirname, "../../../..");
const E2E_DIRS = [
  "apps/admin/tests/e2e",
  "apps/admin/tests/operator",
  "apps/onboarding/tests/e2e",
  "apps/storefront/tests/e2e",
  "apps/storefront/tests/operator",
];

function e2eSources(): ReadonlyArray<[string, string]> {
  const out: Array<[string, string]> = [];
  for (const dir of E2E_DIRS) {
    const abs = path.join(REPO_ROOT, dir);
    for (const name of readdirSync(abs)) {
      if (!name.endsWith(".ts")) continue;
      out.push([`${dir}/${name}`, readFileSync(path.join(abs, name), "utf8")]);
    }
  }
  expect(
    out.length,
    "found no e2e sources to scan — if the specs moved, repoint E2E_DIRS " +
      "rather than deleting this guard, which would silently stop checking",
  ).toBeGreaterThan(20);
  return out;
}

/**
 * Env vars whose value is a credential. A literal fallback for any of these
 * is a committed secret, whatever it happens to contain today.
 */
const CREDENTIAL_ENV = /(PASSWORD|SECRET|API_KEY|KEY_ID|TOKEN|CREDENTIAL)/;

/**
 * Identifiers whose literal value is a credential OR personal data.
 *
 * Wider than CREDENTIAL_ENV by one word — EMAIL — and that word is the whole
 * point of #850. A real Gmail address sat in
 * `apps/admin/tests/operator/admin-verify.spec.ts` as
 * `const CUSTOMER_EMAIL = "…"`, in a PUBLIC repository, belonging to the same
 * account whose password leaked in #844. It is not a secret, so it is not a
 * #844-severity problem — but it is PII that can be scraped for targeting,
 * and nothing here could see it.
 *
 * It was invisible for a structural reason worth naming: both assertions
 * above match a SHAPE that mentions `process.env`. A bare `const X = "…"`
 * mentions no environment variable at all, so neither pattern could ever fire
 * on it, however the identifier was named.
 */
const SENSITIVE_IDENT = /(PASSWORD|SECRET|API_KEY|KEY_ID|TOKEN|CREDENTIAL|EMAIL)/;

/**
 * Domains reserved by RFC 2606 / RFC 6761 for documentation and testing.
 *
 * Broadening the guard to treat an email as sensitive necessarily catches
 * legitimate fixtures — `user@example.com` and friends — so the reserved
 * names are allowed rather than maintaining a per-file allowlist, which
 * would grow one entry at a time until it allowed the next real address.
 * These domains cannot be registered, so a value using one cannot identify
 * a person.
 */
const RESERVED_EXAMPLE = /@(?:[\w-]+\.)*(?:example\.(?:com|org|net)|test|invalid|localhost)$/i;

/** An email-shaped literal. Deliberately loose: the question is whether the
 *  value looks like it could reach a real inbox, not whether it is RFC-valid. */
const EMAIL_SHAPED = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

test("no e2e spec defaults a credential env var to a literal", () => {
  for (const [name, src] of e2eSources()) {
    // process.env.FOO ?? "…" / process.env.FOO || "…", across line breaks,
    // which is how the offender was formatted by prettier.
    for (const m of src.matchAll(
      /process\.env\.([A-Z0-9_]+)\s*(?:\?\?|\|\|)\s*"([^"]*)"/g,
    )) {
      const envName = m[1] ?? "";
      const fallback = m[2] ?? "";
      if (!CREDENTIAL_ENV.test(envName)) continue;
      expect(
        fallback,
        `${name} defaults ${envName} to a non-empty literal. A credential ` +
          `fallback makes the secret part of the repository — and this one is ` +
          `public. Default to "" and let the spec fail on a missing value ` +
          `instead, which is what every other spec here already does.`,
      ).toBe("");
    }
  }
});

test("no e2e spec assigns a credential or a real email to a bare literal", () => {
  // The #850 shape: `const CUSTOMER_EMAIL = "a.real.person@gmail.com"`.
  //
  // Matched on the DECLARATION rather than on `process.env`, because the
  // defining property of this bug is that no environment variable is
  // mentioned. `const`, `let` and `var`, single or double quoted.
  //
  // An empty literal passes: `?? ""` is the fix this guard asks for
  // everywhere else, and a bare `const X = ""` is the same statement.
  for (const [name, src] of e2eSources()) {
    for (const m of src.matchAll(
      /\b(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?::\s*[^=]+)?=\s*(?:"([^"]*)"|'([^']*)')/g,
    )) {
      const ident = m[1] ?? "";
      const value = m[2] ?? m[3] ?? "";
      if (value === "") continue;
      if (!SENSITIVE_IDENT.test(ident.toUpperCase())) continue;

      // An email-shaped value on a reserved documentation domain is a
      // fixture, not a person.
      if (EMAIL_SHAPED.test(value) && RESERVED_EXAMPLE.test(value)) continue;

      expect(
        false,
        `${name} assigns a literal to ${ident}. This repository is PUBLIC. ` +
          `If it is a credential the value is committed; if it is a real ` +
          `email address it is personal data that can be scraped for ` +
          `targeting (#850). Read it from process.env and default to "", ` +
          `with test.skip(!VALUE, "VALUE not set") in the blocks that need ` +
          `it — the idiom every other operator spec here already uses. A ` +
          `fixture address on example.com/.test/.invalid is fine.`,
      ).toBe(true);
    }
  }
});

test("no e2e spec passes a real email address inline to a page interaction", () => {
  // The SECOND occurrence in #850 was not a declaration at all — it was
  // `.fill("a.real.person@gmail.com")` mid-test, in a file that already read
  // CUSTOMER_EMAIL from the environment ten lines from the top. The mechanism
  // was present and simply bypassed, so a guard that only inspects
  // declarations would have caught one of the two and reported the class
  // closed.
  for (const [name, src] of e2eSources()) {
    for (const m of src.matchAll(/(?:"([^"@\s]+@[^"\s]+)"|'([^'@\s]+@[^'\s]+)')/g)) {
      const value = m[1] ?? m[2] ?? "";
      if (!EMAIL_SHAPED.test(value)) continue;
      if (RESERVED_EXAMPLE.test(value)) continue;
      expect(
        false,
        `${name} contains the literal email address ${"<redacted>"}. This ` +
          `repository is PUBLIC — a real address here is personal data ` +
          `(#850). Use a constant read from process.env, or a fixture on a ` +
          `reserved domain such as example.com.`,
      ).toBe(true);
    }
  }
});

test("no e2e spec documents a credential inline in its run instructions", () => {
  for (const [name, src] of e2eSources()) {
    // `KEY=value` in a docblock or comment, as a copy-pasteable run command.
    // A placeholder ($VAR, <your-password>, …) is fine; a bare value is not.
    for (const m of src.matchAll(
      /^\s*(?:\*|\/\/)?\s*([A-Z0-9_]*(?:PASSWORD|SECRET|API_KEY|KEY_ID|TOKEN))=(\S+)/gm,
    )) {
      const envName = m[1] ?? "";
      const value = m[2] ?? "";
      const isPlaceholder =
        /^[$<"'`]/.test(value) || value === "..." || value === "\\";
      expect(
        isPlaceholder,
        `${name} documents ${envName}=${"<redacted>"} with a literal value in a ` +
          `comment. Comments ship in the repository exactly like code does. ` +
          `Use a placeholder such as $${envName} or <${envName.toLowerCase()}>.`,
      ).toBe(true);
    }
  }
});

test("every config that globs tests/e2e/** also globs tests/operator/**", () => {
  // Third instance of one specific mistake: a config or ignore-file names
  // tests/e2e and its twin tests/operator is forgotten. It happened to
  // .gitignore (#844 — a leaked production session, see the tests above),
  // then to two vitest configs that let Playwright specs moved to
  // tests/operator/ get collected by vitest and crash the whole suite
  // (mark8ly#834's e2e-triage follow-up). Both times the fix was symmetric:
  // whatever excludes/ignores tests/e2e/** must do the same for
  // tests/operator/**, in the same file.
  //
  // `git grep -F` only searches tracked files, so this naturally skips
  // node_modules and build output without an explicit exclude list.
  const withE2eGlob = execFileSync(
    "git",
    ["grep", "-l", "-F", "tests/e2e/**"],
    { cwd: REPO_ROOT, encoding: "utf8" },
  )
    .split("\n")
    .filter(Boolean);

  for (const file of withE2eGlob) {
    const src = readFileSync(path.join(REPO_ROOT, file), "utf8");
    expect(
      src.includes("tests/operator/**"),
      `${file} globs "tests/e2e/**" (to exclude or ignore it) without a ` +
        `matching "tests/operator/**" glob in the same file. Playwright ` +
        `specs live under tests/operator/ too (mark8ly#834 moved them ` +
        `there), so anything that excludes/ignores one tree must exclude/ ` +
        `ignore the other, or tests/operator content leaks into whatever ` +
        `this file's tool collects.`,
    ).toBe(true);
  }
});

test("no captured browser session or run artifact is committed under tests/e2e", () => {
  // Literal paths, one per app. A `*` in a git pathspec does NOT cross a
  // `/` — `git ls-files -- "apps/*/tests/e2e/.state/"` returns nothing even
  // while 19 files are tracked there, so the glob form of this assertion
  // passes for the wrong reason. Verified by hand before relying on it.
  const artifactDirs = E2E_DIRS.flatMap((dir) => [
    `${dir}/.state`,
    `${dir}/.audit`,
  ]);
  const tracked = execFileSync(
    "git",
    ["ls-files", "--", ...artifactDirs],
    { cwd: REPO_ROOT, encoding: "utf8" },
  )
    .split("\n")
    .filter(Boolean);

  expect(
    tracked,
    `these run artifacts are tracked. Playwright storageState files hold ` +
      `live session cookies, and the screenshots alongside them are captures ` +
      `of a real signed-in account. They are produced by a run and belong ` +
      `only in a working tree.`,
  ).toEqual([]);
});

test("every e2e run-artifact directory is gitignored, not just some", () => {
  const ignored = readFileSync(path.join(REPO_ROOT, ".gitignore"), "utf8");
  // Both artifact directory names, under BOTH spec trees. tests/operator/
  // is where mark8ly#834 moved the 8 opt-in scripts, and they write exactly
  // the same two directories — a live-host session in .state/ and a live-host
  // audit with screenshots in .audit/. Asserting only the tests/e2e/ pair
  // would recreate the original asymmetry one level up: the operator lines
  // would be present but unguarded, free to be dropped by anyone who did not
  // know why they were there. That is the failure this test exists to
  // prevent, so it has to cover every tree the specs actually live in —
  // extend both lists together if a third tree appears.
  for (const tree of ["e2e", "operator"]) {
    for (const dir of [".audit", ".state"]) {
      expect(
        ignored,
        `.gitignore does not cover apps/*/tests/${tree}/${dir}/. The original ` +
          `leak was exactly this asymmetry: .audit/ was ignored, .state/ was ` +
          `not, and nothing noticed that two directories serving the same ` +
          `purpose were treated differently.`,
      ).toMatch(new RegExp(`apps/\\*/tests/${tree}/\\${dir}/`));
    }
  }
});
