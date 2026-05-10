import { useEffect, useRef } from "react";
import { View } from "react-native";
import { Tabs, useRouter } from "expo-router";
import * as Notifications from "expo-notifications";
import {
  Home as HomeIcon,
  ShoppingBag,
  ShoppingCart,
  UserRound,
} from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { useCartStore } from "@/lib/cart-store";
import { useStorefrontApi } from "@/lib/api-client";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowAlert: true,
    shouldPlaySound: true,
    shouldSetBadge: true,
  }),
});

function usePushSetup() {
  const { user } = useAuth();
  const api = useStorefrontApi();
  const router = useRouter();
  const responseListener = useRef<Notifications.Subscription>();

  useEffect(() => {
    // Only register once we have a customer signed in — push tokens are
    // bound to the customer profile on the server, so registering as a
    // guest would just 401.
    if (!user) return;

    registerForPushNotifications(async (token, platform, deviceId) => {
      await api.post("/push-tokens", {
        token,
        platform,
        device_id: deviceId,
      });
    }).catch(() => {
      // Best-effort — push not being available shouldn't block the app.
    });

    responseListener.current = Notifications.addNotificationResponseReceivedListener(
      (response) => {
        const deepLink = response.notification.request.content.data?.deep_link;
        if (typeof deepLink === "string") {
          router.push(deepLink as never);
        }
      },
    );

    return () => {
      if (responseListener.current) {
        Notifications.removeNotificationSubscription(responseListener.current);
      }
    };
  }, [user, api, router]);
}

const ICON_SIZE = 22;
const ICON_STROKE = 1.75;

function CartIcon({ color }: { color: string }) {
  const count = useCartStore((s) => s.itemCount());
  return (
    <View>
      <ShoppingCart size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
      {count > 0 ? (
        <View
          style={{
            position: "absolute",
            top: -6,
            right: -10,
            minWidth: 18,
            height: 18,
            paddingHorizontal: 4,
            borderRadius: 9,
            backgroundColor: theme.colors.accent,
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Text preset="caption" color="inverse" style={{ fontSize: 10, fontWeight: "700" }}>
            {count > 99 ? "99+" : String(count)}
          </Text>
        </View>
      ) : null}
    </View>
  );
}

export default function TabsLayout() {
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
        headerShown: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Home",
          tabBarIcon: ({ color }) => (
            <HomeIcon size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="shop"
        options={{
          title: "Shop",
          tabBarIcon: ({ color }) => (
            <ShoppingBag size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
      <Tabs.Screen
        name="cart"
        options={{
          title: "Cart",
          tabBarIcon: ({ color }) => <CartIcon color={color} />,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          title: "Account",
          tabBarIcon: ({ color }) => (
            <UserRound size={ICON_SIZE} color={color} strokeWidth={ICON_STROKE} />
          ),
        }}
      />
    </Tabs>
  );
}
