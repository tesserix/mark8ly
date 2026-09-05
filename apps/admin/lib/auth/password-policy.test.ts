import { describe, it, expect } from "vitest";

import {
  PASSWORD_MIN_LENGTH,
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "./password-policy";

describe("password policy", () => {
  it("matches the live Zitadel org policy probed 2026-09-05", () => {
    expect(PASSWORD_MIN_LENGTH).toBe(12);
  });

  it("rejects Test@123_01 — the exact password that failed in production", () => {
    // Eleven characters. The old schema said min(8) and let it through;
    // Zitadel requires twelve and the invitee was told only that we
    // "couldn't finish setting up your account".
    expect("Test@123_01".length).toBe(11);
    const message = validateNewPassword("Test@123_01");
    expect(message).not.toBeNull();
    expect(message).toMatch(/12 characters/);
  });

  it("names every requirement up front rather than one rule at a time", () => {
    for (const password of [
      "short1A!", // too short
      "nouppercase1!", // no uppercase
      "NOLOWERCASE1!", // no lowercase
      "NoNumbersHere!", // no number
      "NoSymbolsHere12", // no symbol
    ]) {
      const message = validateNewPassword(password);
      expect(message, password).not.toBeNull();
      expect(message, password).toMatch(/uppercase/i);
      expect(message, password).toMatch(/lowercase/i);
      expect(message, password).toMatch(/number/i);
      expect(message, password).toMatch(/symbol/i);
      expect(message, password).toMatch(/12 characters/);
    }
  });

  it("accepts a password that satisfies all five rules", () => {
    expect(validateNewPassword("Not-A-Real-Password-1!")).toBeNull();
    expect(validateNewPassword("Still-Not-A-Real-Password-3!")).toBeNull();
  });

  it("counts any non-alphanumeric character as a symbol, as Zitadel does", () => {
    // A client rule narrower than the server's would reject a password
    // Zitadel accepts — the mirror image of the bug being fixed.
    for (const symbol of ["!", "@", "#", "_", "-", " ", "€", "§"]) {
      expect(validateNewPassword(`Abcdefghij1${symbol}`), symbol).toBeNull();
    }
  });

  it("states the whole policy in the helper text shown under the field", () => {
    expect(PASSWORD_REQUIREMENTS_TEXT).toMatch(/12 characters/);
    expect(PASSWORD_REQUIREMENTS_TEXT).toMatch(/uppercase/i);
    expect(PASSWORD_REQUIREMENTS_TEXT).toMatch(/lowercase/i);
    expect(PASSWORD_REQUIREMENTS_TEXT).toMatch(/number/i);
    expect(PASSWORD_REQUIREMENTS_TEXT).toMatch(/symbol/i);
  });
});
