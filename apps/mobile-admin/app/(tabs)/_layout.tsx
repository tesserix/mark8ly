import { useEffect, useRef } from "react";
import { Tabs, useRouter } from "expo-router";
import * as Notifications from "expo-notifications";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { useApiClient } from "@/lib/api-client";
import { TenantGate } from "@/components/TenantGate";
import { Dock } from "@/components/navigation/Dock";

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
    registerForPushNotifications(async (token, platform, deviceId) => {
      await notificationsApi.registerPushToken(token, platform, deviceId);
    }).catch(console.warn);

    responseListener.current =
      Notifications.addNotificationResponseReceivedListener((response) => {
        const deepLink = response.notification.request.content.data?.deep_link;
        if (typeof deepLink === "string") {
          router.push(deepLink as never);
        }
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
