import { Pressable } from "react-native";
import { Stack, useRouter } from "expo-router";
import { Search } from "lucide-react-native";

function SearchHeaderButton() {
  const router = useRouter();
  return (
    <Pressable
      onPress={() => router.push("/(tabs)/browse/search")}
      hitSlop={8}
    >
      <Search size={22} color="#0E0E0C" />
    </Pressable>
  );
}

export default function BrowseLayout() {
  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
      }}
    >
      <Stack.Screen
        name="index"
        options={{
          title: "Browse",
          headerRight: () => <SearchHeaderButton />,
        }}
      />
      <Stack.Screen
        name="search"
        options={{ title: "Search", headerBackTitle: "Browse" }}
      />
      <Stack.Screen
        name="category/[slug]"
        options={{
          title: "Category",
          headerRight: () => <SearchHeaderButton />,
        }}
      />
      <Stack.Screen name="product/[handle]" options={{ title: "" }} />
    </Stack>
  );
}
