import { Stack } from "expo-router";

export default function OrdersLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Orders" }} />
      <Stack.Screen name="[id]" options={{ title: "Order Detail" }} />
    </Stack>
  );
}
