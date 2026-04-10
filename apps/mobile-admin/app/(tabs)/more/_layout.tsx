import { Stack } from "expo-router";
import { theme } from "@/lib/theme";

export default function MoreLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: theme.colors.background }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "More" }} />
      <Stack.Screen name="notifications" options={{ title: "Notifications" }} />
      <Stack.Screen name="account" options={{ title: "Account" }} />
    </Stack>
  );
}
