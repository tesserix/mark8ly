import { useMutation, useQuery } from "@tanstack/react-query";
import { useRouter } from "expo-router";
import { useApiClient, useStoreSlug } from "./use-api-client";
import {
  fetchShippingRates,
  submitCheckout,
  checkCustomerEmail,
  fetchPaymentMethods,
  validateCoupon,
  checkGiftCardBalance,
  createGuestAccount,
  fetchSavedAddresses,
  fetchLoyaltyBalance,
} from "@/lib/storefront-api/checkout";
import { useCartStore } from "@/stores/cart-store";
import { useCheckoutStore } from "@/stores/checkout-store";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import type {
  CheckoutLineItem,
  CheckoutSubmitBody,
  StorefrontAddress,
} from "@repo/mobile-shared/api/storefront-types";

export function useShippingRates(
  address: Omit<StorefrontAddress, "id" | "is_default"> | null,
  cartItems: { productId: string; variantId: string; qty: number }[],
) {
  const client = useApiClient();
  const slug = useStoreSlug();

  const lineItems: CheckoutLineItem[] = cartItems.map((item) => ({
    product_id: item.productId,
    variant_id: item.variantId,
    quantity: item.qty,
  }));

  const isComplete =
    address !== null &&
    address.line1.length > 0 &&
    address.city.length > 0 &&
    address.postal_code.length > 0 &&
    address.country.length > 0;

  return useQuery({
    queryKey: ["store", slug, "shipping-rates", address, lineItems],
    queryFn: () => fetchShippingRates(client, { address: address!, line_items: lineItems }),
    enabled: isComplete && lineItems.length > 0,
    select: (res) => res.data,
  });
}

export function useSubmitCheckout() {
  const client = useApiClient();
  const router = useRouter();
  const clearCart = useCartStore((s) => s.clear);
  const resetCheckout = useCheckoutStore((s) => s.reset);

  return useMutation({
    mutationFn: (body: CheckoutSubmitBody) => submitCheckout(client, body),
    onSuccess: async (result) => {
      await haptics.orderPlaced();
      clearCart();
      router.replace(`/checkout/confirmation/${result.data.order_id}`);
    },
    onError: async () => {
      await haptics.paymentFailed();
    },
  });
}

export function useCheckCustomerEmail() {
  const client = useApiClient();

  return useMutation({
    mutationFn: (email: string) => checkCustomerEmail(client, email),
  });
}

export function usePaymentMethods() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "payment-methods"],
    queryFn: () => fetchPaymentMethods(client),
    select: (res) => res.data,
  });
}

export function useValidateCoupon() {
  const client = useApiClient();

  return useMutation({
    mutationFn: (code: string) => validateCoupon(client, code),
  });
}

export function useCheckGiftCardBalance() {
  const client = useApiClient();

  return useMutation({
    mutationFn: (code: string) => checkGiftCardBalance(client, code),
  });
}

export function useCreateGuestAccount() {
  const client = useApiClient();

  return useMutation({
    mutationFn: (body: { email: string; password: string; name: string }) =>
      createGuestAccount(client, body),
  });
}

export function useSavedAddresses() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "saved-addresses"],
    queryFn: () => fetchSavedAddresses(client),
    select: (res) => res.data,
  });
}

export function useLoyaltyBalance() {
  const client = useApiClient();
  const slug = useStoreSlug();

  return useQuery({
    queryKey: ["store", slug, "loyalty-balance"],
    queryFn: () => fetchLoyaltyBalance(client),
    select: (res) => res.data,
  });
}
