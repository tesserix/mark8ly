import {
  impactAsync,
  notificationAsync,
  ImpactFeedbackStyle,
  NotificationFeedbackType,
} from "expo-haptics";

export const haptics = {
  addToCart: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Light),
  removeFromCart: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Medium),
  quantityChange: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Light),
  wishlistToggle: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Light),
  swipeDelete: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Medium),
  orderPlaced: (): Promise<void> =>
    notificationAsync(NotificationFeedbackType.Success),
  paymentFailed: (): Promise<void> =>
    notificationAsync(NotificationFeedbackType.Error),
  pullToRefresh: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Light),
  checkoutStep: (): Promise<void> => impactAsync(ImpactFeedbackStyle.Light),
};
