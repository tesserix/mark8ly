import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useProductSelection, MAX_SELECTION } from "./useProductSelection";

// Minimal sessionStorage mock for jsdom
const storageMap = new Map<string, string>();
const mockSessionStorage = {
  getItem: vi.fn((key: string) => storageMap.get(key) ?? null),
  setItem: vi.fn((key: string, value: string) => storageMap.set(key, value)),
  removeItem: vi.fn((key: string) => storageMap.delete(key)),
  clear: vi.fn(() => storageMap.clear()),
  get length() { return storageMap.size; },
  key: vi.fn(() => null),
};

Object.defineProperty(globalThis, "sessionStorage", {
  value: mockSessionStorage,
  writable: true,
});

describe("useProductSelection", () => {
  beforeEach(() => {
    storageMap.clear();
    vi.clearAllMocks();
  });

  it("toggle adds and removes ids", () => {
    const { result } = renderHook(() => useProductSelection("store-1"));

    act(() => result.current.toggle("p1"));
    expect(result.current.selectedIds).toEqual(["p1"]);
    expect(result.current.isSelected("p1")).toBe(true);
    expect(result.current.count).toBe(1);

    act(() => result.current.toggle("p1"));
    expect(result.current.selectedIds).toEqual([]);
    expect(result.current.isSelected("p1")).toBe(false);
    expect(result.current.count).toBe(0);
  });

  it("caps at 100 ids — 101st is silently dropped", () => {
    const { result } = renderHook(() => useProductSelection("store-1"));

    act(() => {
      const ids = Array.from({ length: MAX_SELECTION }, (_, i) => `p${i}`);
      result.current.selectAll(ids);
    });

    expect(result.current.count).toBe(MAX_SELECTION);

    // Try adding one more
    act(() => result.current.toggle(`p${MAX_SELECTION}`));
    expect(result.current.count).toBe(MAX_SELECTION);
    expect(result.current.isSelected(`p${MAX_SELECTION}`)).toBe(false);
  });

  it("clearAll empties the selection", () => {
    const { result } = renderHook(() => useProductSelection("store-1"));

    act(() => {
      result.current.toggle("p1");
      result.current.toggle("p2");
    });
    expect(result.current.count).toBe(2);

    act(() => result.current.clearAll());
    expect(result.current.count).toBe(0);
    expect(result.current.selectedIds).toEqual([]);
  });

  it("persists to sessionStorage and rehydrates on re-render", () => {
    const { result, unmount } = renderHook(() => useProductSelection("store-1"));

    act(() => {
      result.current.toggle("p1");
      result.current.toggle("p2");
    });

    expect(mockSessionStorage.setItem).toHaveBeenCalled();
    unmount();

    // Re-render with the same storeId — should rehydrate from storage
    const { result: result2 } = renderHook(() => useProductSelection("store-1"));
    expect(result2.current.selectedIds).toEqual(["p1", "p2"]);
    expect(result2.current.count).toBe(2);
  });

  it("selectAll respects the cap", () => {
    const { result } = renderHook(() => useProductSelection("store-1"));

    const ids = Array.from({ length: 120 }, (_, i) => `p${i}`);
    act(() => result.current.selectAll(ids));

    expect(result.current.count).toBe(MAX_SELECTION);
  });

  it("uses separate storage keys per storeId", () => {
    const { result: r1 } = renderHook(() => useProductSelection("store-a"));
    act(() => r1.current.toggle("p1"));

    const { result: r2 } = renderHook(() => useProductSelection("store-b"));
    expect(r2.current.count).toBe(0);
    expect(r2.current.isSelected("p1")).toBe(false);
  });
});
