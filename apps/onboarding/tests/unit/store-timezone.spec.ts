import { test, expect } from "@playwright/test";

import { storeTimezone } from "../../lib/geo/timezone";

test("country default when the browser is somewhere else", () => {
  // sugarcoat: India store that signed up as America/Chicago.
  expect(storeTimezone("IN", "America/Chicago")).toBe("Asia/Kolkata");
  expect(storeTimezone("AU", "Europe/London")).toBe("Australia/Sydney");
});

test("browser zone wins inside a multi-zone country", () => {
  expect(storeTimezone("AU", "Australia/Perth")).toBe("Australia/Perth");
  expect(storeTimezone("US", "America/Los_Angeles")).toBe("America/Los_Angeles");
  expect(storeTimezone("IN", "Asia/Calcutta")).toBe("Asia/Calcutta");
});

test("unknown country keeps the browser zone, else UTC", () => {
  expect(storeTimezone("ZZ", "Europe/Oslo")).toBe("Europe/Oslo");
  expect(storeTimezone("", "")).toBe("UTC");
});
