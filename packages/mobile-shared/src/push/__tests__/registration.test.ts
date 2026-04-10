import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-notifications", () => ({
  getPermissionsAsync: vi.fn(),
  requestPermissionsAsync: vi.fn(),
  getExpoPushTokenAsync: vi.fn(),
  AndroidImportance: { MAX: 5 },
  setNotificationChannelAsync: vi.fn().mockResolvedValue(null),
}));

vi.mock("expo-device", () => ({
  isDevice: true,
  osName: "iOS",
  deviceName: "iPhone 15",
  modelName: "iPhone",
  osVersion: "17.0",
  deviceYearClass: 2023,
}));

import * as Notifications from "expo-notifications";
import * as Device from "expo-device";
import { registerPushToken, getDeviceId, getPlatform } from "../registration";

describe("registerPushToken", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset Device.isDevice to true by default
    Object.defineProperty(Device, "isDevice", { value: true, writable: true });
  });

  it("returns null when not running on a physical device", async () => {
    Object.defineProperty(Device, "isDevice", {
      value: false,
      writable: true,
    });
    const result = await registerPushToken();
    expect(result).toBeNull();
  });

  it("returns null when permissions are denied and request also denied", async () => {
    (
      Notifications.getPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ status: "denied" });
    (
      Notifications.requestPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ status: "denied" });

    const result = await registerPushToken();
    expect(result).toBeNull();
  });

  it("returns token string when permissions already granted", async () => {
    (
      Notifications.getPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ status: "granted" });
    (
      Notifications.getExpoPushTokenAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ data: "ExponentPushToken[test-token-123]" });

    const result = await registerPushToken();
    expect(result).toBe("ExponentPushToken[test-token-123]");
  });

  it("requests permissions when not yet determined, then returns token", async () => {
    (
      Notifications.getPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ status: "undetermined" });
    (
      Notifications.requestPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ status: "granted" });
    (
      Notifications.getExpoPushTokenAsync as ReturnType<typeof vi.fn>
    ).mockResolvedValue({ data: "ExponentPushToken[new-token]" });

    const result = await registerPushToken();
    expect(Notifications.requestPermissionsAsync).toHaveBeenCalled();
    expect(result).toBe("ExponentPushToken[new-token]");
  });

  it("returns null on error", async () => {
    (
      Notifications.getPermissionsAsync as ReturnType<typeof vi.fn>
    ).mockRejectedValue(new Error("Notifications unavailable"));

    const result = await registerPushToken();
    expect(result).toBeNull();
  });
});

describe("getDeviceId", () => {
  it("returns a non-empty string", () => {
    const id = getDeviceId();
    expect(typeof id).toBe("string");
    expect(id.length).toBeGreaterThan(0);
  });
});

describe("getPlatform", () => {
  it("returns ios, android, or unknown", () => {
    const platform = getPlatform();
    expect(["ios", "android", "unknown"]).toContain(platform);
  });
});
