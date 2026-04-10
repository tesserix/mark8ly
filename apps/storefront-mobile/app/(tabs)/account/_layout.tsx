import { Stack } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";

export default function AccountLayout() {
  const theme = useTheme();

  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: theme.background },
        headerTintColor: theme.text,
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
