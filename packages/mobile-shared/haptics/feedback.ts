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

/**
 * Haptics are unavailable on simulators and on some Android hardware, where
 * expo-haptics either rejects or throws synchronously. Feedback is never
 * worth failing an interaction over, so every admin trigger is fire-and-forget.
 */
function safe(fn: () => Promise<void> | void): () => Promise<void> {
  return async () => {
    try {
      await fn();
    } catch {
      // Intentionally swallowed — haptic feedback is never user-critical and
      // must never surface as an unhandled rejection.
    }
  };
}

/**
 * Admin-side feedback vocabulary. Named for the moment, not the waveform, so
 * call sites read as intent. Separate from `haptics` above, which is
 * storefront-shaped (addToCart / checkoutStep / …) and used by another app.
 */
export const adminHaptics = {
  /** Tab change, filter chip, segmented control. */
  selectionChanged: safe(() => Haptics.selectionAsync()),
  /** A swipe gesture crosses its action threshold. Fires once per crossing. */
  swipeThreshold: safe(() =>
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  ),
  /** A long-press opens an action sheet. */
  menuOpen: safe(() =>
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  ),
  /** Order fulfilled, review approved, save succeeded. */
  actionSucceeded: safe(() =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success),
  ),
  /** Action failed or validation blocked. */
  actionFailed: safe(() =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error),
  ),
} as const;
