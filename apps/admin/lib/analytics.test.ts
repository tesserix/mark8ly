import { describe, it, expect, vi, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";

import { trackEvent, useAnalyticsTenant } from "./analytics";

type Win = { op?: (...args: unknown[]) => void };

afterEach(() => {
  delete (window as unknown as Win).op;
  vi.useRealTimers();
});

describe("trackEvent", () => {
  it("is a no-op when analytics isn't loaded", () => {
    expect(() => trackEvent("domain_added")).not.toThrow();
  });

  it("forwards the event to OpenPanel when loaded", () => {
    const op = vi.fn();
    (window as unknown as Win).op = op;
    trackEvent("payment_provider_saved", { provider: "stripe" });
    expect(op).toHaveBeenCalledWith("track", "payment_provider_saved", { provider: "stripe" });
  });
});

describe("useAnalyticsTenant", () => {
  it("waits for OpenPanel, then identifies by tenant ID only, once", () => {
    vi.useFakeTimers();
    renderHook(() => useAnalyticsTenant("t-1", "s-1"));

    const op = vi.fn();
    vi.advanceTimersByTime(500);
    (window as unknown as Win).op = op;
    vi.advanceTimersByTime(500);
    vi.advanceTimersByTime(2000);

    expect(op).toHaveBeenCalledWith("identify", { profileId: "t-1" });
    expect(op).toHaveBeenCalledWith("setGlobalProperties", { tenant_id: "t-1", store_id: "s-1" });
    expect(op).toHaveBeenCalledTimes(2);
  });

  it("does nothing without a tenant", () => {
    vi.useFakeTimers();
    const op = vi.fn();
    (window as unknown as Win).op = op;
    renderHook(() => useAnalyticsTenant(undefined, undefined));
    vi.advanceTimersByTime(1000);
    expect(op).not.toHaveBeenCalled();
  });
});
