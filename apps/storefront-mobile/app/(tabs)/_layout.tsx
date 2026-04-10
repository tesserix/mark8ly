import { Tabs } from "expo-router";
import { Home, Search, ShoppingBag, User } from "lucide-react-native";
import { View, Text, StyleSheet } from "react-native";

function CartIcon({ color, size }: { color: string; size: number }) {
  // Badge count hardcoded to 0; will be wired in Phase 5
  const count = 0;
  return (
    <View>
      <ShoppingBag color={color} size={size} />
      {count > 0 && (
        <View style={styles.badge}>
          <Text style={styles.badgeText}>{count > 99 ? "99+" : count}</Text>
        </View>
      )}
    </View>
  );
}

export default function TabLayout() {
  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: "#0E0E0C",
        tabBarInactiveTintColor: "#999999",
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
        options={{
          title: "Home",
          tabBarIcon: ({ color, size }) => <Home color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="browse"
        options={{
          title: "Browse",
          headerShown: false,
          tabBarIcon: ({ color, size }) => <Search color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="cart"
        options={{
          title: "Cart",
          tabBarIcon: ({ color, size }) => <CartIcon color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          title: "Account",
          headerShown: false,
          tabBarIcon: ({ color, size }) => <User color={color} size={size} />,
        }}
      />
    </Tabs>
  );
}

const styles = StyleSheet.create({
  badge: {
    position: "absolute",
    right: -6,
    top: -4,
    backgroundColor: "#0E0E0C",
    borderRadius: 8,
    minWidth: 16,
    height: 16,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 3,
  },
  badgeText: {
    color: "#F7F6F2",
    fontSize: 10,
    fontWeight: "700",
  },
});
