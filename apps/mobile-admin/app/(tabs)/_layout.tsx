import { Tabs } from "expo-router";

export default function TabLayout() {
  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: "#0E0E0C",
        tabBarInactiveTintColor: "#0E0E0C80",
        tabBarStyle: {
          backgroundColor: "#FFFFFF",
          borderTopColor: "#0E0E0C10",
          borderTopWidth: 0.5,
        },
        headerStyle: { backgroundColor: "#F7F6F2" },
        headerTintColor: "#0E0E0C",
        headerShadowVisible: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{ title: "Dashboard" }}
      />
      <Tabs.Screen
        name="orders"
        options={{ title: "Orders", headerShown: false }}
      />
      <Tabs.Screen
        name="products"
        options={{ title: "Products", headerShown: false }}
      />
      <Tabs.Screen
        name="customers"
        options={{ title: "Customers", headerShown: false }}
      />
      <Tabs.Screen
        name="more"
        options={{ title: "More", headerShown: false }}
      />
    </Tabs>
  );
}
