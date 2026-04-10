import { Pressable, StyleSheet } from "react-native";
import { Heart } from "lucide-react-native";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useRouter } from "expo-router";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import {
  useIsWishlisted,
  useAddToWishlist,
  useRemoveFromWishlist,
} from "@/lib/hooks/use-wishlist";

interface WishlistButtonProps {
  productId: string;
  initialWishlisted?: boolean;
}

export function WishlistButton({
  productId,
  initialWishlisted: _initialWishlisted = false,
}: WishlistButtonProps) {
  const { user } = useAuth();
  const router = useRouter();
  const wishlisted = useIsWishlisted(productId);
  const addMutation = useAddToWishlist();
  const removeMutation = useRemoveFromWishlist();
  const busy = addMutation.isPending || removeMutation.isPending;

  const handlePress = async () => {
    if (!user) {
      router.push("/(auth)/login");
      return;
    }
    await haptics.wishlistToggle();
    if (wishlisted) {
      removeMutation.mutate(productId);
    } else {
      addMutation.mutate(productId);
    }
  };

  return (
    <Pressable
      style={styles.button}
      onPress={handlePress}
      hitSlop={8}
      disabled={busy}
      accessibilityRole="button"
      accessibilityLabel={wishlisted ? "Remove from wishlist" : "Add to wishlist"}
    >
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
