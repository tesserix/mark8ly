import { Stack } from "expo-router";

export default function BrowseLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
      }}
    >
      <Stack.Screen name="index" options={{ title: "Browse" }} />
      <Stack.Screen name="search" options={{ title: "Search" }} />
      <Stack.Screen name="category/[slug]" options={{ title: "Category" }} />
      <Stack.Screen name="product/[handle]" options={{ title: "Product" }} />
    </Stack>
  );
}
