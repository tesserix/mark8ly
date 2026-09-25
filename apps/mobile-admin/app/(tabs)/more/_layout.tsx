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
 * `more/settings` USED to own a bare <Stack> with no index route and no
 * anchor. Popping its only screen therefore left that navigator holding
 * nothing at all — an empty stack cannot render, so Back fell out to the
 * Dashboard and the More tab was left with a stack that showed nothing when
 * navigated to. Reported from a device as "notification screen takes back to
 * dashboard not prev screen and more screen becomes inaccessible".
 *
 * That layout is now gone. The settings screens are children of THIS stack,
 * which is anchored, so `index` is always beneath them. Their URLs are
 * unchanged — expo-router derives paths from the directory tree, not from
 * where the navigators sit — and `headerShown: false` is inherited from the
 * screenOptions below, which is all the deleted layout contributed.
 */
export const unstable_settings = { initialRouteName: 'index' };

export default function MoreLayout() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="marketing" />
      <Stack.Screen name="account" />
      <Stack.Screen name="support" />
    </Stack>
  );
}
