import { Stack } from "expo-router";

export default function CustomersLayout() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="index" />
      <Stack.Screen name="[id]" />
      <Stack.Screen name="reviews/index" />
      <Stack.Screen name="reviews/[id]" />
    </Stack>
  );
}
