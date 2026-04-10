import * as Notifications from "expo-notifications";
import * as Device from "expo-device";

export type Platform = "ios" | "android" | "unknown";

export async function registerPushToken(): Promise<string | null> {
  if (!Device.isDevice) {
    return null;
  }

  try {
    const { status: existingStatus } =
      await Notifications.getPermissionsAsync();

    let finalStatus = existingStatus;

    if (existingStatus !== "granted") {
      const { status } = await Notifications.requestPermissionsAsync();
      finalStatus = status;
    }

    if (finalStatus !== "granted") {
      return null;
    }

    const tokenData = await Notifications.getExpoPushTokenAsync();
    return tokenData.data;
  } catch {
    return null;
  }
}

export function getDeviceId(): string {
  const name = Device.deviceName ?? "unknown-device";
  const model = Device.modelName ?? "unknown-model";
  const os = Device.osName ?? "unknown-os";
  const version = Device.osVersion ?? "0";
  const yearClass = Device.deviceYearClass?.toString() ?? "0";
  return `${os}-${model}-${name}-${version}-${yearClass}`
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "");
}

export function getPlatform(): Platform {
  const osName = (Device.osName ?? "").toLowerCase();
  if (osName === "ios") return "ios";
  if (osName === "android") return "android";
  return "unknown";
}
