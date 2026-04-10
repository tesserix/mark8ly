import { Platform } from "react-native";
import { router } from "expo-router";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { extractDeepLinkRoute } from "@repo/mobile-shared/deep-links/validator";
import { registerPushToken } from "@/lib/storefront-api/push";
import type { ApiClient } from "@/lib/storefront-api/client-provider";
import * as Notifications from "expo-notifications";

interface PushCleanup {
  dispose: () => void;
}

export async function setupPushNotifications(
  client: ApiClient,
): Promise<PushCleanup> {
  const registration = await registerForPushNotifications();

  if (registration?.token) {
    const deviceId = registration.deviceId ?? "unknown";
    const platform = Platform.OS === "ios" ? "ios" as const : "android" as const;

    try {
      await registerPushToken(client, registration.token, deviceId, platform);
    } catch {
      // Token registration is best-effort; the app still works without it.
    }
  }

  const receivedSubscription = Notifications.addNotificationReceivedListener(
    (_notification) => {
      // Notification received while app is foregrounded.
      // No-op by default; the OS notification banner handles display.
    },
  );

  const responseSubscription =
    Notifications.addNotificationResponseReceivedListener((response) => {
      const data = response.notification.request.content.data as
        | Record<string, unknown>
        | undefined;

      if (!data) return;

      const deepLink =
        typeof data.deep_link === "string" ? data.deep_link : undefined;
      const url = typeof data.url === "string" ? data.url : undefined;
      const target = deepLink ?? url;

      if (!target) return;

      const route = extractDeepLinkRoute(target);
      if (route) {
        router.push(route as never);
      }
    });

  return {
    dispose: () => {
      receivedSubscription.remove();
      responseSubscription.remove();
    },
  };
}
