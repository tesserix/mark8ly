import type { ApiClient } from "./client-provider";

interface PushTokenResponse {
  data: { id: string };
}

export function registerPushToken(
  client: ApiClient,
  token: string,
  deviceId: string,
  platform: "ios" | "android",
): Promise<PushTokenResponse> {
  return client.post<PushTokenResponse>("/push-tokens", {
    token,
    device_id: deviceId,
    platform,
  });
}

export function deletePushToken(
  client: ApiClient,
  tokenId: string,
): Promise<void> {
  return client.delete(`/push-tokens/${tokenId}`);
}
