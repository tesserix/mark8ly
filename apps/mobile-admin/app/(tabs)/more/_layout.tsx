import { Stack } from "expo-router";

export default function MoreLayout() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="notifications" />
      <Stack.Screen name="account" />
      <Stack.Screen name="security" />
      <Stack.Screen name="support" />
    </Stack>
  );
}
