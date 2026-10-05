import { test, expect } from "@playwright/test";

import {
  isExpired,
  isTagged,
  mergeAcquisition,
  parseStoredAcquisition,
  touchFromLocation,
  RETENTION_DAYS,
  type Acquisition,
} from "../../lib/acquisition";

/**
 * Issue #992 — the browser-side half of attribution. These are the pure
 * rules; the e2e golden path proves the record reaches the server and the
 * server's own tests prove what it keeps.
 */

const T0 = new Date("2026-10-05T10:00:00Z");
const later = (days: number) => new Date(T0.getTime() + days * 24 * 3600 * 1000);

test.describe("touchFromLocation", () => {
  test("keeps the five UTM values, strips the query from the landing path", () => {
    const touch = touchFromLocation(
      "https://mark8ly.com/ecommerce-for-makers?utm_source=newsletter&utm_medium=email&utm_campaign=spring&utm_content=hero&utm_term=makers&token=abc&email=a%40b.c",
      "https://news.example.org/issue/12?session=secret",
      T0,
    );
    expect(touch).toEqual({
      utm_source: "newsletter",
      utm_medium: "email",
      utm_campaign: "spring",
      utm_content: "hero",
      utm_term: "makers",
      landing_path: "/ecommerce-for-makers",
      referrer: "https://news.example.org/issue/12",
      captured_at: "2026-10-05T10:00:00.000Z",
    });
    expect(JSON.stringify(touch)).not.toMatch(/token|abc|a@b\.c|secret/);
  });

  test("an untagged arrival records path, referrer and time, and nothing invented", () => {
    const touch = touchFromLocation("https://mark8ly.com/", "", T0);
    expect(touch).toEqual({ landing_path: "/", captured_at: "2026-10-05T10:00:00.000Z" });
    expect(isTagged(touch)).toBe(false);
  });

  test("a same-host referrer is not a referral", () => {
    const touch = touchFromLocation(
      "https://mark8ly.com/onboarding",
      "https://mark8ly.com/",
      T0,
    );
    expect(touch.referrer).toBeUndefined();
  });

  test("non-http referrers are dropped", () => {
    const touch = touchFromLocation("https://mark8ly.com/", "javascript:alert(1)", T0);
    expect(touch.referrer).toBeUndefined();
  });
});

test.describe("mergeAcquisition", () => {
  const campaign = touchFromLocation(
    "https://mark8ly.com/?utm_source=ig&utm_campaign=launch",
    "https://instagram.com/p/1",
    T0,
  );

  test("first arrival sets both touches", () => {
    const record = mergeAcquisition(undefined, campaign, T0);
    expect(record).toEqual({ first: campaign, last: campaign });
  });

  test("a later tagged arrival updates last and keeps first", () => {
    const stored = mergeAcquisition(undefined, campaign, T0);
    const retarget = touchFromLocation(
      "https://mark8ly.com/?utm_source=google&utm_campaign=retarget",
      "",
      later(3),
    );
    const record = mergeAcquisition(stored, retarget, later(3));
    expect(record.first).toEqual(campaign);
    expect(record.last).toEqual(retarget);
  });

  test("an untagged return never overwrites a campaign", () => {
    const stored = mergeAcquisition(undefined, campaign, T0);
    const direct = touchFromLocation("https://mark8ly.com/onboarding", "", later(1));
    expect(mergeAcquisition(stored, direct, later(1))).toBe(stored);
  });

  test(`memory lasts ${RETENTION_DAYS} days from first, then starts over`, () => {
    const stored = mergeAcquisition(undefined, campaign, T0);
    const direct = touchFromLocation("https://mark8ly.com/", "", later(RETENTION_DAYS + 1));
    expect(isExpired(stored, later(RETENTION_DAYS - 1))).toBe(false);
    expect(isExpired(stored, later(RETENTION_DAYS + 1))).toBe(true);
    expect(mergeAcquisition(stored, direct, later(RETENTION_DAYS + 1))).toEqual({
      first: direct,
      last: direct,
    });
  });
});

test.describe("parseStoredAcquisition", () => {
  test("reads back only allowlisted string fields", () => {
    const raw = JSON.stringify({
      first: { utm_source: "ig", landing_path: "/", password: "x", nested: { a: 1 } },
      last: { utm_campaign: 42 },
      extra: true,
    });
    expect(parseStoredAcquisition(raw)).toEqual({
      first: { utm_source: "ig", landing_path: "/" },
      last: undefined,
    });
  });

  test("malformed storage reads as absent", () => {
    for (const raw of [null, "", "{", "[]", "null", JSON.stringify({ first: {} })]) {
      expect(parseStoredAcquisition(raw)).toBeUndefined();
    }
  });

  test("round-trips a merged record", () => {
    const record: Acquisition = mergeAcquisition(
      undefined,
      touchFromLocation("https://mark8ly.com/?utm_source=x", "", T0),
      T0,
    );
    expect(parseStoredAcquisition(JSON.stringify(record))).toEqual(record);
  });
});
