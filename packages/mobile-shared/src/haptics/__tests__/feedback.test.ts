import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-haptics", () => ({
  impactAsync: vi.fn().mockResolvedValue(undefined),
  notificationAsync: vi.fn().mockResolvedValue(undefined),
  ImpactFeedbackStyle: {
    Light: "Light",
    Medium: "Medium",
    Heavy: "Heavy",
  },
  NotificationFeedbackType: {
    Success: "Success",
    Warning: "Warning",
    Error: "Error",
  },
}));

import * as ExpoHaptics from "expo-haptics";
import { haptics } from "../feedback";

describe("haptics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("addToCart calls impactAsync with Light", async () => {
    await haptics.addToCart();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Light
    );
  });

  it("removeFromCart calls impactAsync with Medium", async () => {
    await haptics.removeFromCart();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Medium
    );
  });

  it("quantityChange calls impactAsync with Light", async () => {
    await haptics.quantityChange();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Light
    );
  });

  it("wishlistToggle calls impactAsync with Light", async () => {
    await haptics.wishlistToggle();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Light
    );
  });

  it("swipeDelete calls impactAsync with Medium", async () => {
    await haptics.swipeDelete();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Medium
    );
  });

  it("orderPlaced calls notificationAsync with Success", async () => {
    await haptics.orderPlaced();
    expect(ExpoHaptics.notificationAsync).toHaveBeenCalledWith(
      ExpoHaptics.NotificationFeedbackType.Success
    );
  });

  it("paymentFailed calls notificationAsync with Error", async () => {
    await haptics.paymentFailed();
    expect(ExpoHaptics.notificationAsync).toHaveBeenCalledWith(
      ExpoHaptics.NotificationFeedbackType.Error
    );
  });

  it("pullToRefresh calls impactAsync with Light", async () => {
    await haptics.pullToRefresh();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Light
    );
  });

  it("checkoutStep calls impactAsync with Light", async () => {
    await haptics.checkoutStep();
    expect(ExpoHaptics.impactAsync).toHaveBeenCalledWith(
      ExpoHaptics.ImpactFeedbackStyle.Light
    );
  });
});
