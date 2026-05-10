import { useMutation, useQuery } from "@tanstack/react-query";
import type {
  CheckoutLineItem,
  CheckoutResult,
  CheckoutSubmitBody,
  PaymentMethod,
  ShippingRate,
  StorefrontAddress,
} from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";

interface PaymentMethodsResponse {
  items: PaymentMethod[];
}

export function usePaymentMethods() {
  const api = useStorefrontApi();
  return useQuery<PaymentMethodsResponse>({
    queryKey: ["payment-methods"],
    queryFn: () => api.get<PaymentMethodsResponse>("/payment-methods"),
    staleTime: 5 * 60_000,
  });
}

interface ShippingRatesResponse {
  items: ShippingRate[];
}

interface ShippingRatesInput {
  shipping_address: Omit<StorefrontAddress, "id" | "is_default">;
  line_items: CheckoutLineItem[];
}

export function useShippingRates() {
  const api = useStorefrontApi();
  return useMutation({
    mutationFn: (body: ShippingRatesInput) =>
      api.post<ShippingRatesResponse>("/checkout/shipping-rates", body),
  });
}

export function useValidateCoupon() {
  const api = useStorefrontApi();
  return useMutation({
    mutationFn: (code: string) =>
      api.post<{ valid: boolean; discount_amount: string; message?: string }>(
        "/coupons/validate",
        { code },
      ),
  });
}

export function useSubmitCheckout() {
  const api = useStorefrontApi();
  return useMutation({
    mutationFn: (body: CheckoutSubmitBody) =>
      api.post<CheckoutResult>("/checkout/submit", body),
  });
}
