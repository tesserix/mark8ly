import { useEffect, useRef } from "react";
import { Tabs, useRouter } from "expo-router";
import * as Notifications from "expo-notifications";
import { createApiClient } from "@repo/mobile-shared/api/client";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { theme } from "@/lib/theme";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

function useApiClient() {
  const { getToken } = useAuth();
  const activeStore = useTenantStore((s) => s.activeStore);
  return createApiClient({
    baseUrl: "https://api.mark8ly.com", // TODO: read from config
    getToken,
    getStoreId: () => activeStore?.id ?? null,
  });
}

function usePushSetup() {
  const router = useRouter();
  const responseListener = useRef<Notifications.Subscription>();
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
      if (responseListener.current) {
        Notifications.removeNotificationSubscription(responseListener.current);
      }
    };
  }, []);
}

export default function TabLayout() {
  usePushSetup();

  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: theme.colors.text,
        tabBarInactiveTintColor: `${theme.colors.text}80`,
        tabBarStyle: {
          backgroundColor: theme.colors.elevated,
          borderTopColor: `${theme.colors.text}10`,
          borderTopWidth: 0.5,
        },
        headerStyle: { backgroundColor: theme.colors.background },
        headerTintColor: theme.colors.text,
        headerShadowVisible: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{ title: "Dashboard" }}
      />
      <Tabs.Screen
        name="orders"
        options={{ title: "Orders", headerShown: false }}
      />
      <Tabs.Screen
        name="products"
        options={{ title: "Products", headerShown: false }}
      />
      <Tabs.Screen
        name="customers"
        options={{ title: "Customers", headerShown: false }}
      />
      <Tabs.Screen
        name="more"
        options={{ title: "More", headerShown: false }}
      />
    </Tabs>
  );
}
