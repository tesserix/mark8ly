import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import {
  PASSWORD_MIN_LENGTH,
  PASSWORD_REQUIREMENTS_MESSAGE,
  validateNewPassword,
} from "../../lib/auth/password-policy";

// A merchant reported: the field says "At least 12 characters, with an
// uppercase letter, a lowercase letter, a number, and a symbol", and then
// submitting a short password answered "Password must be at least 8
// characters". Two different numbers, one field.
//
// The cause was a stale `min(8)` in each form's zod resolver, which ran
// BEFORE the correct policy check and short-circuited it. password-policy.ts
// claims to be the only copy of these rules on the TypeScript side; these
// tests make that claim enforceable.

const FORMS = [
  "../../components/onboarding/SetPasswordForm.tsx",
  "../../../admin/components/auth/AcceptInviteForm.tsx",
  "../../../admin/components/auth/ResetPasswordForm.tsx",
];

describe("password resolvers agree with the policy", () => {
  it.each(FORMS)("%s hardcodes no length of its own", (rel) => {
    const src = readFileSync(join(__dirname, rel), "utf8");
    // Strip comments: they legitimately discuss the old min(8).
    const code = src
      .replace(/\/\*[\s\S]*?\*\//g, "")
      .split("\n")
      .filter((l) => !l.trim().startsWith("//"))
      .join("\n");

    // Threshold 6, not 2: a name or slug field may legitimately carry a
    // small minimum. Anything >= 6 in an auth form is a password length,
    // and a password length belongs in password-policy.ts.
    const minCalls = [...code.matchAll(/\.min\(\s*(\d+)/g)].map((m) => Number(m[1]));
    const lengthish = minCalls.filter((n) => n >= 6);
    expect(
      lengthish,
      `found hardcoded minimum(s) ${lengthish.join(", ")} — read PASSWORD_MIN_LENGTH from password-policy.ts instead`,
    ).toEqual([]);

    expect(code).not.toMatch(/at least 8 characters/i);
  });
});

describe("validateNewPassword", () => {
  it("rejects a short password with the full requirements, not a length", () => {
    const msg = validateNewPassword("Ab1!x");
    expect(msg).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
    expect(msg).not.toMatch(/8 characters/);
    expect(msg).toContain(String(PASSWORD_MIN_LENGTH));
  });

  it.each([
    ["nouppercase1!xxxx", "no uppercase"],
    ["NOLOWERCASE1!XXXX", "no lowercase"],
    ["NoDigitsHere!xxxx", "no number"],
    ["NoSymbolHere1xxxx", "no symbol"],
    ["Ab1!", "too short"],
  ])("rejects %j (%s) with the same complete message", (pw) => {
    expect(validateNewPassword(pw)).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
  });

  it("accepts a password meeting every rule", () => {
    expect(validateNewPassword("CorrectHorse1!")).toBeNull();
  });

  it("rejects an 11-character password — the original incident", () => {
    // A merchant's 11-char password passed min(8), then Zitadel refused it.
    expect(validateNewPassword("Abcdefgh1!x")).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
    expect(validateNewPassword("Abcdefgh1!xy")).toBeNull();
  });
});
