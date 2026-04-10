import { Stack } from "expo-router";

export default function ProductsLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Products" }} />
      <Stack.Screen name="[id]" options={{ title: "Product" }} />
      <Stack.Screen name="new" options={{ title: "New Product" }} />
    </Stack>
  );
}
