import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  PASSWORD_MIN_LENGTH,
  PASSWORD_REQUIREMENTS_MESSAGE,
  validateNewPassword,
} from "../../lib/auth/password-policy";

/**
 * A merchant reported: the field says "At least 12 characters, with an
 * uppercase letter, a lowercase letter, a number, and a symbol", and then
 * submitting a short password answered "Password must be at least 8
 * characters". Two different numbers, one field.
 *
 * The cause was a stale `min(8)` in each form's zod resolver, which ran
 * BEFORE the correct policy check and short-circuited it. password-policy.ts
 * claims to be the only copy of these rules on the TypeScript side; these
 * tests make that claim enforceable.
 */

const FORMS = [
  "../../components/onboarding/SetPasswordForm.tsx",
  "../../../admin/components/auth/AcceptInviteForm.tsx",
  "../../../admin/components/auth/ResetPasswordForm.tsx",
];

/**
 * Strip comments before matching. Without this the check is worthless: a
 * comment discussing the old min(8) satisfies a substring search.
 */
function code(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .split("\n")
    .filter((l) => !l.trim().startsWith("//"))
    .join("\n");
}

test.describe("password resolvers agree with the policy", () => {
  for (const rel of FORMS) {
    test(`${rel} hardcodes no length of its own`, () => {
      const src = code(readFileSync(path.join(__dirname, rel), "utf8"));

      // Threshold 6, not 2: a name or slug field may legitimately carry a
      // small minimum. Anything >= 6 in an auth form is a password length,
      // and a password length belongs in password-policy.ts.
      const mins = [...src.matchAll(/\.min\(\s*(\d+)/g)].map((m) => Number(m[1]));
      const lengthish = mins.filter((n) => n >= 6);
      expect(
        lengthish,
        `found hardcoded minimum(s) ${lengthish.join(", ")} — read PASSWORD_MIN_LENGTH from password-policy.ts instead`,
      ).toEqual([]);

      expect(src.toLowerCase()).not.toContain("at least 8 characters");
    });
  }
});

test.describe("validateNewPassword", () => {
  test("rejects a short password with the full requirements, not a length", () => {
    const msg = validateNewPassword("Ab1!x");
    expect(msg).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
    expect(msg).not.toContain("8 characters");
    expect(msg).toContain(String(PASSWORD_MIN_LENGTH));
  });

  const rejected: Array<[string, string]> = [
    ["nouppercase1!xxxx", "no uppercase"],
    ["NOLOWERCASE1!XXXX", "no lowercase"],
    ["NoDigitsHere!xxxx", "no number"],
    ["NoSymbolHere1xxxx", "no symbol"],
    ["Ab1!", "too short"],
  ];
  for (const [pw, why] of rejected) {
    test(`rejects ${why} with the same complete message`, () => {
      expect(validateNewPassword(pw)).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
    });
  }

  test("accepts a password meeting every rule", () => {
    expect(validateNewPassword("CorrectHorse1!")).toBeNull();
  });

  test("rejects an 11-character password — the original incident", () => {
    // A merchant's 11-char password passed min(8), then Zitadel refused it.
    expect(validateNewPassword("Abcdefgh1!x")).toBe(PASSWORD_REQUIREMENTS_MESSAGE);
    expect(validateNewPassword("Abcdefgh1!xy")).toBeNull();
  });
});
