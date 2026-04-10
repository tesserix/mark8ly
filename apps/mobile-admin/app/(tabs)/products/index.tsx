import { View, Text, StyleSheet } from "react-native";

export default function ProductsScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.text}>Products</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#F7F6F2", justifyContent: "center", alignItems: "center" },
  text: { color: "#0E0E0C", fontSize: 16 },
});
