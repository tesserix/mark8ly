import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => {
  cleanup();
});

// Web Storage, which this environment supplies only on some Node versions.
//
// `window.localStorage` reads as `undefined` under Node 26 even though the
// document has a real origin and jsdom implements Storage:
//
//   - Node 26 defines `globalThis.localStorage` itself, as an accessor that
//     returns `undefined` unless the process was started with
//     `--localstorage-file` (it warns exactly that).
//   - vitest's jsdom environment populates globals only for keys not already
//     present. The key IS present — as Node's always-undefined accessor — so
//     jsdom's real Storage never lands on it.
//
// Measured, because the boundary is narrower than "new Node":
//
//   v22.19.0  absent              MobileAppPrompt passes
//   v24.20.0  absent              MobileAppPrompt passes
//   v26.5.0   present, undefined  MobileAppPrompt fails 8 tests
//
// So this is NOT what #857 is about — CI runs 22 today and is unaffected
// either way. It is here so the suite is runnable on 26, where
// `MobileAppPrompt.test.tsx` otherwise dies in its `afterEach` on
// `localStorage.clear()` and takes `cleanup()` with it, leaving later tests
// to fail with "Found multiple elements" — failures that look like duplicate
// renders and are not.
//
// Ported from tesserix-home#627, which fixed the identical thing in the
// console.
//
// A real Map-backed Storage rather than no-op stubs: these tests SET a value
// and assert the component reads it back ("stays dismissed across a remount",
// "scopes dismissal per tenant"), so stubs that dropped writes would convert
// hard failures into quiet false passes.
//
// Guarded on the VALUE, not the key: `??=` and `"localStorage" in globalThis`
// both see Node's accessor and skip, leaving the bug in place.
if (typeof globalThis !== "undefined" && !(globalThis as { localStorage?: unknown }).localStorage) {
  const store = new Map<string, string>();
  const storage: Storage = {
    get length() {
      return store.size;
    },
    clear: () => store.clear(),
    getItem: (key: string) => (store.has(key) ? store.get(key)! : null),
    key: (index: number) => [...store.keys()][index] ?? null,
    removeItem: (key: string) => void store.delete(key),
    setItem: (key: string, value: string) => void store.set(String(key), String(value)),
  };
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    writable: true,
    value: storage,
  });
}
