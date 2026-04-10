import { describe, it, expect, vi, beforeEach } from "vitest";

const { mockImpactAsync, mockNotificationAsync } = vi.hoisted(() => ({
  mockImpactAsync: vi.fn().mockResolvedValue(undefined),
  mockNotificationAsync: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("expo-haptics", () => ({
  impactAsync: mockImpactAsync,
  notificationAsync: mockNotificationAsync,
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

import { haptics } from "../feedback";
import * as Haptics from "expo-haptics";

describe("haptics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("impact feedback", () => {
    it("addToCart calls impactAsync with Light", async () => {
      await haptics.addToCart();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Light,
      );
    });

    it("removeFromCart calls impactAsync with Medium", async () => {
      await haptics.removeFromCart();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Medium,
      );
    });

    it("quantityChange calls impactAsync with Light", async () => {
      await haptics.quantityChange();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Light,
      );
    });

    it("wishlistToggle calls impactAsync with Light", async () => {
      await haptics.wishlistToggle();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Light,
      );
    });

    it("swipeDelete calls impactAsync with Medium", async () => {
      await haptics.swipeDelete();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Medium,
      );
    });

    it("pullToRefresh calls impactAsync with Light", async () => {
      await haptics.pullToRefresh();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Light,
      );
    });

    it("checkoutStep calls impactAsync with Light", async () => {
      await haptics.checkoutStep();
      expect(mockImpactAsync).toHaveBeenCalledOnce();
      expect(mockImpactAsync).toHaveBeenCalledWith(
        Haptics.ImpactFeedbackStyle.Light,
      );
    });
  });

  describe("notification feedback", () => {
    it("orderPlaced calls notificationAsync with Success", async () => {
      await haptics.orderPlaced();
      expect(mockNotificationAsync).toHaveBeenCalledOnce();
      expect(mockNotificationAsync).toHaveBeenCalledWith(
        Haptics.NotificationFeedbackType.Success,
      );
    });

    it("paymentFailed calls notificationAsync with Error", async () => {
      await haptics.paymentFailed();
      expect(mockNotificationAsync).toHaveBeenCalledOnce();
      expect(mockNotificationAsync).toHaveBeenCalledWith(
        Haptics.NotificationFeedbackType.Error,
      );
    });
  });

  describe("impact vs notification separation", () => {
    it("impact methods do not call notificationAsync", async () => {
      await haptics.addToCart();
      await haptics.removeFromCart();
      await haptics.quantityChange();
      expect(mockNotificationAsync).not.toHaveBeenCalled();
    });

    it("notification methods do not call impactAsync", async () => {
      await haptics.orderPlaced();
      await haptics.paymentFailed();
      expect(mockImpactAsync).not.toHaveBeenCalled();
    });
  });
});
