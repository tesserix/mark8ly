import { Stack, usePathname } from "expo-router";
import { useMemo } from "react";
import { useTheme } from "@/lib/theme/theme-provider";
import { CheckoutProgress } from "@/components/CheckoutProgress";
import type { CheckoutStep } from "@/stores/checkout-store";

const STEP_MAP: Record<string, CheckoutStep> = {
  "/checkout/details": 1, "/checkout/shipping": 2, "/checkout/payment": 3, "/checkout/review": 4,
};

function CheckoutHeader() {
  const pathname = usePathname();
  const step = useMemo(() => STEP_MAP[pathname] ?? null, [pathname]);
  if (!step) return null;
  return <CheckoutProgress currentStep={step} />;
}

export default function CheckoutLayout() {
  const theme = useTheme();
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: theme.background }, headerTintColor: theme.text, headerShadowVisible: false, title: "Checkout", headerBottom: CheckoutHeader }}>
      <Stack.Screen name="details" options={{ title: "Checkout" }} />
      <Stack.Screen name="shipping" options={{ title: "Checkout" }} />
      <Stack.Screen name="payment" options={{ title: "Checkout" }} />
      <Stack.Screen name="review" options={{ title: "Checkout" }} />
      <Stack.Screen name="confirmation/[id]" options={{ title: "Order Confirmed", headerBackVisible: false }} />
    </Stack>
  );
}
