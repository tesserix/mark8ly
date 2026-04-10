import { Stack } from "expo-router";
import { theme } from "@/lib/theme";

export default function CustomersLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: theme.colors.background }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "Customers" }} />
      <Stack.Screen name="[id]" options={{ title: "Customer" }} />
    </Stack>
  );
}
