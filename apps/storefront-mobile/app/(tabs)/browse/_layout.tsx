import { Pressable } from "react-native";
import { Stack, useRouter } from "expo-router";
import { Search } from "lucide-react-native";
import { useTheme } from "@/lib/theme/theme-provider";

function SearchHeaderButton() {
  const router = useRouter();
  const theme = useTheme();
  return (
    <Pressable
      onPress={() => router.push("/(tabs)/browse/search")}
      hitSlop={8}
      accessibilityRole="button"
      accessibilityLabel="Search products"
    >
      <Search size={22} color={theme.text} />
    </Pressable>
  );
}

export default function BrowseLayout() {
  const theme = useTheme();

  return (
    <Stack
      screenOptions={{
        headerStyle: { backgroundColor: theme.background },
        headerTintColor: theme.text,
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
