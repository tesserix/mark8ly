import { useEffect, useRef } from "react";
import { Tabs, useRouter } from "expo-router";
import * as Notifications from "expo-notifications";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { tokenStorage } from "@repo/mobile-shared/auth/token-storage";
import { useApiClient } from "@/lib/api-client";
import { TenantGate } from "@/components/TenantGate";
import { Dock } from "@/components/navigation/Dock";

// Push payloads carry an in-app path to open. Only navigate when it targets a
// known admin section — an unvalidated value straight from a notification must
// never drive arbitrary navigation. The storefront deep-link validator
// (packages/mobile-shared/deep-links) only knows storefront routes
// (account/*, browse/*), so admin uses this minimal prefix allowlist instead.
const ALLOWED_DEEP_LINK_SEGMENTS = [
  "orders",
  "products",
  "customers",
  "more",
  "notifications",
] as const;

function safeDeepLinkPath(value: unknown): string | null {
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  if (trimmed === "" || trimmed.includes("..")) return null;
  const path = trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
  const firstSegment = path.split("/")[1];
  if (!firstSegment) return null;
  return ALLOWED_DEEP_LINK_SEGMENTS.some((segment) => segment === firstSegment)
    ? path
    : null;
}

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    // shouldShowAlert is deprecated in expo-notifications 56; the banner/list
    // pair replaces it and both are required by NotificationBehavior.
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

function usePushSetup() {
  const router = useRouter();
  const responseListener = useRef<Notifications.EventSubscription | undefined>(undefined);
  const client = useApiClient();
  const notificationsApi = createNotificationsApi(client);

  useEffect(() => {
    // Respect the device-level opt-out from Settings > Notifications: skip
    // registration entirely when the user has turned push off.
    (async () => {
      if (!(await tokenStorage.getPushEnabled())) return;
      await registerForPushNotifications(async (token, platform, deviceId) => {
        const res = await notificationsApi.registerPushToken(token, platform, deviceId);
        if (res?.id) await tokenStorage.setPushTokenId(res.id);
      });
    })().catch(console.warn);

    responseListener.current =
      Notifications.addNotificationResponseReceivedListener((response) => {
        const target = safeDeepLinkPath(
          response.notification.request.content.data?.deep_link,
        );
        // Fail safe: an unrecognised or malformed target is ignored rather
        // than pushed, so a bad payload can never crash or misroute the app.
        if (target) router.push(target as never);
      });

    return () => {
      // expo-notifications 56 removed Notifications.removeNotificationSubscription;
      // calling it threw. Subscriptions remove themselves.
      responseListener.current?.remove();
    };
  }, []);
}

export default function TabLayout() {
  return (
    <TenantGate>
      <TabsInner />
    </TenantGate>
  );
}

function TabsInner() {
  usePushSetup();

  // The floating Dock (components/navigation/Dock) is the tab bar; it renders
  // its own icons/labels from the route name, so per-screen tabBarIcon options
  // aren't needed. Titles feed the accessibility label.
  return (
    <Tabs
      tabBar={(props) => <Dock {...props} />}
      screenOptions={{ headerShown: false }}
    >
      <Tabs.Screen name="index" options={{ title: "Dashboard" }} />
      <Tabs.Screen name="orders" options={{ title: "Orders" }} />
      <Tabs.Screen name="products" options={{ title: "Products" }} />
      <Tabs.Screen name="customers" options={{ title: "Customers" }} />
      <Tabs.Screen name="more" options={{ title: "More" }} />
    </Tabs>
  );
}
