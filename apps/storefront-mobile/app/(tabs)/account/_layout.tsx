import { Stack } from "expo-router";

export default function AccountLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
      }}
    >
      <Stack.Screen name="index" options={{ title: "Account" }} />
      <Stack.Screen name="orders" options={{ title: "Orders" }} />
      <Stack.Screen name="orders/[id]" options={{ title: "Order" }} />
      <Stack.Screen name="addresses" options={{ title: "Addresses" }} />
      <Stack.Screen name="wishlist" options={{ title: "Wishlist" }} />
      <Stack.Screen name="loyalty" options={{ title: "Loyalty" }} />
      <Stack.Screen name="reviews" options={{ title: "Reviews" }} />
    </Stack>
  );
}
