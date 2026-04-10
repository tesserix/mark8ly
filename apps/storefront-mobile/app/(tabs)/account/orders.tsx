import { View, Text, StyleSheet } from "react-native";

export default function OrdersScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.text}>Orders</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#F7F6F2", alignItems: "center", justifyContent: "center" },
  text: { fontSize: 18, color: "#0E0E0C" },
});
