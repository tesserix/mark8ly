import { Stack } from "expo-router";

/**
 * ANCHOR ROUTE. Without this, entering this stack at a NESTED route leaves the
 * stack holding only that route — so Back exits the tab entirely and the list
 * screen is unreachable. Reported as "product details back goes to dashboard
 * directly, no way to view the product list".
 *
 * Five call sites enter a tab stack at a nested route, so every one of them hit
 * this: the Dashboard's NEEDS YOU queue pushes `orders/{id}`, `products/{id}`,
 * `customers/reviews/{id}` and `more/settings/tickets/{id}` (lib/queue.ts), and
 * notifications.tsx pushes `more/settings/notification-settings`. Push-payload
 * deep links (app/(tabs)/_layout.tsx ALLOWED_DEEP_LINK_SEGMENTS) and any
 * external `mark8ly-admin://` link are the same shape.
 *
 * `initialRouteName` is the key expo-router 56.2.14 actually reads — verified
 * in node_modules/expo-router/build/getRoutesCore.js:415, not assumed.
 *
 * `more/settings` deliberately has NO anchor: it owns no index route, and its
 * screens are leaves reached through the More menu, which anchors instead.
 */
export const unstable_settings = { initialRouteName: 'index' };

export default function CustomersLayout() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="[id]" />
      <Stack.Screen name="reviews/index" />
      <Stack.Screen name="reviews/[id]" />
    </Stack>
  );
}
