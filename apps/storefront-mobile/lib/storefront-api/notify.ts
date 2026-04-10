import type { ApiClient } from "./client-provider";

interface NotifyResponse {
  success: boolean;
}

export function subscribeNotify(
  client: ApiClient,
  productId: string,
  notifyType: string,
): Promise<NotifyResponse> {
  return client.post<NotifyResponse>("/products/_/notify-me", {
    product_id: productId,
    notify_type: notifyType,
  });
}

export function unsubscribeNotify(
  client: ApiClient,
  productId: string,
  notifyType: string,
): Promise<NotifyResponse> {
  return client.delete<NotifyResponse>(
    `/products/_/notify-me/${notifyType}?product_id=${productId}`,
  );
}
