import { Stack } from "expo-router";
import { theme } from "@/lib/theme";

export default function ProductsLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: theme.colors.background }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Products" }} />
      <Stack.Screen name="[id]" options={{ title: "Product" }} />
      <Stack.Screen name="new" options={{ title: "New Product" }} />
    </Stack>
  );
}
