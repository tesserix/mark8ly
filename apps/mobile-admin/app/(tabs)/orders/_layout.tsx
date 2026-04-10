import { Stack } from "expo-router";
import { theme } from "@/lib/theme";

export default function OrdersLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: theme.colors.background }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Orders" }} />
      <Stack.Screen name="[id]" options={{ title: "Order Detail" }} />
    </Stack>
  );
}
