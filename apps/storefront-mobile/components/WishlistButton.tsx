import { useState } from "react";
import { Pressable, StyleSheet } from "react-native";
import { Heart } from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { haptics } from "@repo/mobile-shared/haptics/feedback";

interface WishlistButtonProps {
  productId: string;
  initialWishlisted?: boolean;
}

export function WishlistButton({
  productId: _productId,
  initialWishlisted = false,
}: WishlistButtonProps) {
  const { user } = useAuth();
  const router = useRouter();
  const [wishlisted, setWishlisted] = useState(initialWishlisted);

  const handlePress = async () => {
    if (!user) {
      router.push("/(auth)/login");
      return;
    }
    await haptics.wishlistToggle();
    setWishlisted((prev) => !prev);
    // TODO: wire to wishlist API in Phase 6
  };

  return (
    <Pressable style={styles.button} onPress={handlePress} hitSlop={8}>
      <Heart
        size={24}
        color={wishlisted ? "#0E0E0C" : "#666666"}
        fill={wishlisted ? "#0E0E0C" : "none"}
      />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  button: {
    width: 40,
    height: 40,
    borderRadius: 20,
    backgroundColor: "#FFFFFF",
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.08,
    shadowRadius: 4,
    elevation: 2,
  },
});
