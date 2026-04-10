import type { ApiClient } from "./client-provider";
import type {
  CheckoutLineItem,
  CheckoutSubmitBody,
  CheckoutResult,
  ShippingRate,
  PaymentMethod,
  StorefrontAddress,
} from "@repo/mobile-shared/api/storefront-types";

interface ShippingRatesRequest {
  address: Omit<StorefrontAddress, "id" | "is_default">;
  line_items: CheckoutLineItem[];
}

interface ShippingRatesResponse {
  data: ShippingRate[];
}

interface CheckoutResponse {
  data: CheckoutResult;
}

interface PaymentMethodsResponse {
  data: PaymentMethod[];
}

interface CouponValidateResponse {
  data: {
    valid: boolean;
    discount_amount: string;
    discount_type: "fixed" | "percent";
  };
}

interface GiftCardBalanceResponse {
  data: {
    balance: string;
    currency_code: string;
  };
}

export function fetchShippingRates(
  client: ApiClient,
  body: ShippingRatesRequest,
): Promise<ShippingRatesResponse> {
  return client.post<ShippingRatesResponse>("/checkout/shipping-rates", body);
}

export function submitCheckout(
  client: ApiClient,
  body: CheckoutSubmitBody,
): Promise<CheckoutResponse> {
  return client.post<CheckoutResponse>("/checkout/submit", body);
}

export function checkCustomerEmail(
  client: ApiClient,
  email: string,
): Promise<{ ok: boolean; status: number }> {
  return client.head(`/account/check-email?email=${encodeURIComponent(email)}`);
}

export function fetchPaymentMethods(
  client: ApiClient,
): Promise<PaymentMethodsResponse> {
  return client.get<PaymentMethodsResponse>("/payment-methods");
}

export function validateCoupon(
  client: ApiClient,
  code: string,
): Promise<CouponValidateResponse> {
  return client.post<CouponValidateResponse>("/coupons/validate", { code });
}

export function checkGiftCardBalance(
  client: ApiClient,
  code: string,
): Promise<GiftCardBalanceResponse> {
  return client.post<GiftCardBalanceResponse>("/gift-cards/balance", { code });
}

export function createGuestAccount(
  client: ApiClient,
  body: { email: string; password: string; name: string },
): Promise<{ data: { id: string } }> {
  return client.post<{ data: { id: string } }>("/account/register", body);
}

export function fetchSavedAddresses(
  client: ApiClient,
): Promise<{ data: StorefrontAddress[] }> {
  return client.get<{ data: StorefrontAddress[] }>("/account/addresses");
}

export function fetchLoyaltyBalance(
  client: ApiClient,
): Promise<{ data: { points_balance: number; currency_per_point: number } }> {
  return client.get<{
    data: { points_balance: number; currency_per_point: number };
  }>("/loyalty/me");
}
