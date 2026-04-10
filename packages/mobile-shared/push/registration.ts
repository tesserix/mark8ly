import * as Notifications from "expo-notifications";
import { Platform } from "react-native";
import * as Device from "expo-device";
import { tokenStorage } from "../auth/token-storage";

export async function registerForPushNotifications(
  registerFn: (token: string, platform: string, deviceId: string) => Promise<void>,
): Promise<string | null> {
  if (!Device.isDevice) return null;

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  const pushToken = await Notifications.getExpoPushTokenAsync();
  const platform = Platform.OS === "ios" ? "ios" : "android";

  let deviceId = await tokenStorage.getDeviceId();
  if (!deviceId) {
    deviceId = `${platform}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    await tokenStorage.setDeviceId(deviceId);
  }

  await registerFn(pushToken.data, platform, deviceId);

  return pushToken.data;
}
