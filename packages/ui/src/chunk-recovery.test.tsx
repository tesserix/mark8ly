import { describe, expect, it } from "vitest";

import { isChunkLoadFailure } from "./chunk-recovery";

// Reloading the page is disruptive, so this predicate is the safety gate:
// it must catch the real stale-asset shapes and nothing else.
describe("isChunkLoadFailure", () => {
  it.each([
    "ChunkLoadError: Loading chunk 402 failed.",
    "Loading chunk app/page failed.",
    "Loading CSS chunk 12 failed.",
    "Failed to fetch dynamically imported module: https://mark8ly.com/_next/static/x.js",
    "error loading dynamically imported module",
    "Importing a module script failed.",
  ])("treats %j as a stale-asset failure", (msg) => {
    expect(isChunkLoadFailure(msg)).toBe(true);
  });

  it.each([
    "TypeError: Cannot read properties of undefined (reading 'id')",
    "NetworkError when attempting to fetch resource.",
    "Hydration failed because the initial UI does not match",
    "Request failed with status code 500",
    "chunk", // substring alone must not trigger a reload
  ])("does not reload for %j", (msg) => {
    expect(isChunkLoadFailure(msg)).toBe(false);
  });

  it("handles empty and missing input", () => {
    expect(isChunkLoadFailure("")).toBe(false);
    expect(isChunkLoadFailure(undefined)).toBe(false);
    expect(isChunkLoadFailure(null)).toBe(false);
  });
});
