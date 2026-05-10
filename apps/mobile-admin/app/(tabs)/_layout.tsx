import { useEffect, useRef } from "react";
import { Tabs, useRouter } from "expo-router";
import * as Notifications from "expo-notifications";
import {
  LayoutDashboard,
  ShoppingBag,
  Package,
  Users,
  MoreHorizontal,
} from "lucide-react-native";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { useApiClient } from "@/lib/api-client";
import { TenantGate } from "@/components/TenantGate";
import { theme } from "@/lib/theme";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

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

const ICON_SIZE = 22;
const ICON_STROKE = 1.75;

export default function TabLayout() {
  return (
    <TenantGate>
      <TabsInner />
    </TenantGate>
  );
}

function TabsInner() {
  usePushSetup();

  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: theme.colors.text,
        tabBarInactiveTintColor: theme.colors.textTertiary,
        tabBarStyle: {
          backgroundColor: theme.colors.elevated,
          borderTopColor: theme.colors.hairline,
          borderTopWidth: theme.hairline,
          height: 64,
          paddingTop: 6,
          paddingBottom: 10,
        },
        tabBarLabelStyle: {
          fontFamily: theme.fonts.sans,
          fontSize: 11,
          fontWeight: "600",
          letterSpacing: 0.3,
          marginTop: 2,
        },
        tabBarItemStyle: {
          paddingVertical: 4,
        },
        headerShown: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Dashboard",
          tabBarIcon: ({ color }) => (
            <LayoutDashboard size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="orders"
        options={{
          title: "Orders",
          tabBarIcon: ({ color }) => (
            <ShoppingBag size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="products"
        options={{
          title: "Products",
          tabBarIcon: ({ color }) => (
            <Package size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="customers"
        options={{
          title: "Customers",
          tabBarIcon: ({ color }) => (
            <Users size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="more"
        options={{
          title: "More",
          tabBarIcon: ({ color }) => (
            <MoreHorizontal size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
    </Tabs>
  );
}
