import type { ApiClient } from "./client-provider";

export interface OrderLineItem {
  id: string;
  product_id: string;
  product_title: string;
  variant_title: string;
  quantity: number;
  unit_price: string;
  total_price: string;
  thumbnail_url?: string;
}

export interface OrderEvent {
  id: string;
  status: string;
  description: string;
  created_at: string;
}

export interface OrderSummary {
  id: string;
  order_number: string;
  status: string;
  total: string;
  currency_code: string;
  item_count: number;
  created_at: string;
}

export interface OrderDetail {
  id: string;
  order_number: string;
  status: string;
  created_at: string;
  currency_code: string;
  line_items: OrderLineItem[];
  shipping_address: {
    line1: string;
    line2?: string;
    city: string;
    state: string;
    postal_code: string;
    country: string;
  };
  shipping_method: string;
  tracking_number?: string;
  payment_method: string;
  subtotal: string;
  shipping_total: string;
  discount_total: string;
  tax_total: string;
  total: string;
  events: OrderEvent[];
}

interface OrderListResponse {
  data: OrderSummary[];
  meta: {
    total: number;
    page: number;
    page_size: number;
  };
}

interface OrderDetailResponse {
  data: OrderDetail;
}

export function listOrders(
  client: ApiClient,
  page: number,
  pageSize: number,
): Promise<OrderListResponse> {
  return client.get<OrderListResponse>(
    `/orders?page=${page}&page_size=${pageSize}`,
  );
}

export function getOrder(
  client: ApiClient,
  id: string,
): Promise<OrderDetailResponse> {
  return client.get<OrderDetailResponse>(`/orders/${id}`);
}
