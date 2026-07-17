import { useCallback, useEffect, useState } from "react";
import * as Notifications from "expo-notifications";
import { createNotificationsApi } from "@repo/mobile-shared/api/notifications";
import { registerForPushNotifications } from "@repo/mobile-shared/push/registration";
import { tokenStorage } from "@repo/mobile-shared/auth/token-storage";
import { useApiClient } from "@/lib/api-client";

type PermissionStatus = "granted" | "denied" | "undetermined";

export interface SetPushResult {
  ok: boolean;
  /** Set when enabling failed because the OS permission was withheld. */
  reason?: "permission";
}

function normalize(status: Notifications.PermissionStatus): PermissionStatus {
  if (status === "granted") return "granted";
  if (status === "denied") return "denied";
  return "undetermined";
}

/**
 * Backs the Settings > Notifications toggle. Combines the device-level opt-out
 * (stored locally) with the live OS permission, and — on enable — registers
 * the push token, on disable — deletes the server registration so the backend
 * stops targeting this device.
 */
export function usePushPreference() {
  const client = useApiClient();
  const [enabled, setEnabled] = useState(true);
  const [permission, setPermission] = useState<PermissionStatus>("undetermined");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [pref, perms] = await Promise.all([
        tokenStorage.getPushEnabled(),
        Notifications.getPermissionsAsync(),
      ]);
      if (cancelled) return;
      setEnabled(pref);
      setPermission(normalize(perms.status));
      setLoading(false);
    })().catch(() => {
      if (!cancelled) setLoading(false);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const setPushEnabled = useCallback(
    async (next: boolean): Promise<SetPushResult> => {
      const notificationsApi = createNotificationsApi(client);
      setBusy(true);
      try {
        if (next) {
          const current = await Notifications.getPermissionsAsync();
          let status = current.status;
          if (status !== "granted") {
            const requested = await Notifications.requestPermissionsAsync();
            status = requested.status;
          }
          setPermission(normalize(status));
          if (status !== "granted") {
            return { ok: false, reason: "permission" };
          }
          await registerForPushNotifications(async (token, platform, deviceId) => {
            const res = await notificationsApi.registerPushToken(token, platform, deviceId);
            if (res?.id) await tokenStorage.setPushTokenId(res.id);
          });
          await tokenStorage.setPushEnabled(true);
          setEnabled(true);
          return { ok: true };
        }

        await tokenStorage.setPushEnabled(false);
        setEnabled(false);
        const id = await tokenStorage.getPushTokenId();
        if (id) {
          // Best-effort: even if the delete fails, the local opt-out already
          // stops re-registration on next launch.
          await notificationsApi.deletePushToken(id).catch(() => undefined);
          await tokenStorage.clearPushTokenId();
        }
        return { ok: true };
      } finally {
        setBusy(false);
      }
    },
    [client],
  );

  return { enabled, permission, loading, busy, setPushEnabled };
}
