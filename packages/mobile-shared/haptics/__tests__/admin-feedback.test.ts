import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockImpactAsync,
  mockNotificationAsync,
  mockSelectionAsync,
} = vi.hoisted(() => ({
  mockImpactAsync: vi.fn().mockResolvedValue(undefined),
  mockNotificationAsync: vi.fn().mockResolvedValue(undefined),
  mockSelectionAsync: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("expo-haptics", () => ({
  impactAsync: mockImpactAsync,
  notificationAsync: mockNotificationAsync,
  selectionAsync: mockSelectionAsync,
  ImpactFeedbackStyle: { Light: "Light", Medium: "Medium", Heavy: "Heavy" },
  NotificationFeedbackType: {
    Success: "Success",
    Warning: "Warning",
    Error: "Error",
  },
}));

import { adminHaptics } from "../feedback";
import * as Haptics from "expo-haptics";

describe("adminHaptics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockImpactAsync.mockResolvedValue(undefined);
    mockNotificationAsync.mockResolvedValue(undefined);
    mockSelectionAsync.mockResolvedValue(undefined);
  });

  it("selectionChanged calls selectionAsync", async () => {
    await adminHaptics.selectionChanged();
    expect(mockSelectionAsync).toHaveBeenCalledOnce();
  });

  it("swipeThreshold calls impactAsync with Light", async () => {
    await adminHaptics.swipeThreshold();
    expect(mockImpactAsync).toHaveBeenCalledWith(
      Haptics.ImpactFeedbackStyle.Light,
    );
  });

  it("menuOpen calls impactAsync with Medium", async () => {
    await adminHaptics.menuOpen();
    expect(mockImpactAsync).toHaveBeenCalledWith(
      Haptics.ImpactFeedbackStyle.Medium,
    );
  });

  it("actionSucceeded calls notificationAsync with Success", async () => {
    await adminHaptics.actionSucceeded();
    expect(mockNotificationAsync).toHaveBeenCalledWith(
      Haptics.NotificationFeedbackType.Success,
    );
  });

  it("actionFailed calls notificationAsync with Error", async () => {
    await adminHaptics.actionFailed();
    expect(mockNotificationAsync).toHaveBeenCalledWith(
      Haptics.NotificationFeedbackType.Error,
    );
  });

  it("never rejects when the platform has no haptics engine", async () => {
    mockSelectionAsync.mockRejectedValueOnce(new Error("Unsupported"));
    await expect(adminHaptics.selectionChanged()).resolves.toBeUndefined();
  });

  it("never rejects when an impact call throws synchronously", async () => {
    mockImpactAsync.mockImplementationOnce(() => {
      throw new Error("Unsupported");
    });
    await expect(adminHaptics.swipeThreshold()).resolves.toBeUndefined();
  });
});
