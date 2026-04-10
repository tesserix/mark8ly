import { Stack } from "expo-router";

export default function MoreLayout() {
  return (
    <Stack screenOptions={{ headerStyle: { backgroundColor: "#F7F6F2" }, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ title: "More" }} />
      <Stack.Screen name="notifications" options={{ title: "Notifications" }} />
      <Stack.Screen name="account" options={{ title: "Account" }} />
    </Stack>
  );
}
