import { Stack } from "expo-router";

export default function CheckoutLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
        title: "Checkout",
      }}
    >
      <Stack.Screen name="details" options={{ title: "Checkout" }} />
      <Stack.Screen name="shipping" options={{ title: "Checkout" }} />
      <Stack.Screen name="payment" options={{ title: "Checkout" }} />
      <Stack.Screen name="review" options={{ title: "Checkout" }} />
      <Stack.Screen name="confirmation/[id]" options={{ title: "Order Confirmed", headerBackVisible: false }} />
    </Stack>
  );
}
