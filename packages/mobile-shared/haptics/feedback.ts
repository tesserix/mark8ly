import * as Haptics from "expo-haptics";

export const haptics = {
  addToCart: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  removeFromCart: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  quantityChange: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  wishlistToggle: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  swipeDelete: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  orderPlaced: () =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success),
  paymentFailed: () =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error),
  pullToRefresh: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  checkoutStep: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
} as const;
