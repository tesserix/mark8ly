import { useCallback, useRef, useMemo } from "react";
import {
  View,
  Text,
  Image,
  Pressable,
  StyleSheet,
  Animated,
  PanResponder,
  Alert,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { haptics } from "@repo/mobile-shared/haptics/feedback";

interface StockInfo {
  available: number;
  status: string;
}

interface CartItemProps {
  variantId: string;
  title: string;
  imageUrl?: string;
  priceAmount: number;
  currencyCode: string;
  qty: number;
  stockInfo?: StockInfo;
  onUpdateQty: (variantId: string, qty: number) => void;
  onRemove: (variantId: string) => void;
  onSaveForLater: (variantId: string) => void;
}

const SWIPE_THRESHOLD = -80;
const ACTION_WIDTH = 80;

export function CartItem({
  variantId,
  title,
  imageUrl,
  priceAmount,
  currencyCode,
  qty,
  stockInfo,
  onUpdateQty,
  onRemove,
  onSaveForLater,
}: CartItemProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const translateX = useRef(new Animated.Value(0)).current;

  const panResponder = useRef(
    PanResponder.create({
      onMoveShouldSetPanResponder: (_, gesture) =>
        Math.abs(gesture.dx) > 10 && Math.abs(gesture.dy) < 10,
      onPanResponderMove: (_, gesture) => {
        if (gesture.dx < 0) {
          translateX.setValue(Math.max(gesture.dx, -ACTION_WIDTH * 2));
        }
      },
      onPanResponderRelease: (_, gesture) => {
        if (gesture.dx < SWIPE_THRESHOLD * 2) {
          Animated.spring(translateX, {
            toValue: -ACTION_WIDTH * 2,
            useNativeDriver: true,
          }).start();
        } else if (gesture.dx < SWIPE_THRESHOLD) {
          Animated.spring(translateX, {
            toValue: -ACTION_WIDTH,
            useNativeDriver: true,
          }).start();
        } else {
          Animated.spring(translateX, {
            toValue: 0,
            useNativeDriver: true,
          }).start();
        }
      },
    }),
  ).current;

  const isOutOfStock = stockInfo?.status === "out_of_stock";
  const isLowStock =
    stockInfo && stockInfo.available > 0 && stockInfo.available < qty;
  const lineTotal = (priceAmount * qty).toFixed(2);

  const handleIncrement = useCallback(async () => {
    await haptics.quantityChange();
    onUpdateQty(variantId, qty + 1);
  }, [variantId, qty, onUpdateQty]);

  const handleDecrement = useCallback(async () => {
    if (qty <= 1) {
      Alert.alert("Remove item", `Remove "${title}" from your cart?`, [
        { text: "Cancel", style: "cancel" },
        {
          text: "Remove",
          style: "destructive",
          onPress: async () => {
            await haptics.removeFromCart();
            onRemove(variantId);
          },
        },
      ]);
      return;
    }
    await haptics.quantityChange();
    onUpdateQty(variantId, qty - 1);
  }, [variantId, qty, title, onUpdateQty, onRemove]);

  const handleRemoveAction = useCallback(async () => {
    await haptics.swipeDelete();
    onRemove(variantId);
    Animated.spring(translateX, {
      toValue: 0,
      useNativeDriver: true,
    }).start();
  }, [variantId, onRemove, translateX]);

  const handleSaveAction = useCallback(() => {
    onSaveForLater(variantId);
    Animated.spring(translateX, {
      toValue: 0,
      useNativeDriver: true,
    }).start();
  }, [variantId, onSaveForLater, translateX]);

  return (
    <View style={styles.wrapper}>
      {/* Swipe actions behind */}
      <View style={styles.actionsContainer}>
        <Pressable
          style={styles.saveAction}
          onPress={handleSaveAction}
          accessibilityRole="button"
          accessibilityLabel="Save for later"
        >
          <Text style={styles.actionText}>Save</Text>
        </Pressable>
        <Pressable
          style={styles.deleteAction}
          onPress={handleRemoveAction}
          accessibilityRole="button"
          accessibilityLabel={`Remove ${title}`}
        >
          <Text style={styles.actionText}>Delete</Text>
        </Pressable>
      </View>

      {/* Main row */}
      <Animated.View
        style={[styles.row, { transform: [{ translateX }] }]}
        {...panResponder.panHandlers}
      >
        <View style={styles.imageContainer}>
          {imageUrl ? (
            <Image
              source={{ uri: imageUrl }}
              style={styles.image}
              accessibilityLabel={title}
            />
          ) : (
            <View style={styles.imagePlaceholder}>
              <Text style={styles.imagePlaceholderText}>No img</Text>
            </View>
          )}
        </View>

        <View style={styles.details}>
          <Text style={styles.title} numberOfLines={2}>
            {title}
          </Text>

          <Text style={styles.unitPrice}>
            {currencyCode} {priceAmount.toFixed(2)}
          </Text>

          {isOutOfStock && (
            <View style={styles.stockBanner}>
              <Text style={styles.stockBannerTextDanger}>Out of stock</Text>
            </View>
          )}

          {isLowStock && !isOutOfStock && (
            <View style={styles.stockBannerWarn}>
              <Text style={styles.stockBannerTextWarn}>
                Only {stockInfo.available} available
              </Text>
            </View>
          )}

          <View style={styles.bottomRow}>
            <View style={styles.stepper}>
              <Pressable
                style={styles.stepperButton}
                onPress={handleDecrement}
                accessibilityRole="button"
                accessibilityLabel={`Decrease quantity of ${title}`}
              >
                <Text style={styles.stepperText}>-</Text>
              </Pressable>
              <Text style={styles.qtyText}>{qty}</Text>
              <Pressable
                style={[
                  styles.stepperButton,
                  isOutOfStock && styles.stepperDisabled,
                ]}
                onPress={handleIncrement}
                disabled={isOutOfStock}
                accessibilityRole="button"
                accessibilityLabel={`Increase quantity of ${title}`}
              >
                <Text
                  style={[
                    styles.stepperText,
                    isOutOfStock && styles.stepperTextDisabled,
                  ]}
                >
                  +
                </Text>
              </Pressable>
            </View>

            <Text style={styles.lineTotal}>
              {currencyCode} {lineTotal}
            </Text>
          </View>
        </View>
      </Animated.View>
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  wrapper: {
    overflow: "hidden",
  },
  actionsContainer: {
    position: "absolute",
    right: 0,
    top: 0,
    bottom: 0,
    flexDirection: "row",
  },
  saveAction: {
    backgroundColor: theme.accent,
    width: ACTION_WIDTH,
    alignItems: "center",
    justifyContent: "center",
  },
  deleteAction: {
    backgroundColor: "#8B2500",
    width: ACTION_WIDTH,
    alignItems: "center",
    justifyContent: "center",
  },
  actionText: {
    color: theme.elevated,
    fontSize: 13,
    fontWeight: "600",
  },
  row: {
    flexDirection: "row",
    backgroundColor: theme.elevated,
    padding: 16,
    gap: 12,
  },
  imageContainer: {
    width: 60,
    height: 60,
    borderRadius: 6,
    overflow: "hidden",
    backgroundColor: theme.background,
  },
  image: {
    width: 60,
    height: 60,
  },
  imagePlaceholder: {
    width: 60,
    height: 60,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: theme.border,
  },
  imagePlaceholderText: {
    fontSize: 10,
    color: theme.textSecondary,
  },
  details: {
    flex: 1,
    gap: 4,
  },
  title: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.text,
    lineHeight: 18,
  },
  unitPrice: {
    fontSize: 13,
    color: theme.textSecondary,
  },
  stockBanner: {
    backgroundColor: "#FDE8E8",
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
    alignSelf: "flex-start",
  },
  stockBannerTextDanger: {
    fontSize: 11,
    fontWeight: "600",
    color: "#8B2500",
  },
  stockBannerWarn: {
    backgroundColor: "#FEF3E2",
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
    alignSelf: "flex-start",
  },
  stockBannerTextWarn: {
    fontSize: 11,
    fontWeight: "600",
    color: "#B87333",
  },
  bottomRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginTop: 4,
  },
  stepper: {
    flexDirection: "row",
    alignItems: "center",
    borderWidth: 1,
    borderColor: theme.border,
    borderRadius: 6,
  },
  stepperButton: {
    width: 32,
    height: 32,
    alignItems: "center",
    justifyContent: "center",
  },
  stepperDisabled: {
    opacity: 0.3,
  },
  stepperText: {
    fontSize: 16,
    fontWeight: "600",
    color: theme.text,
  },
  stepperTextDisabled: {
    color: theme.textSecondary,
  },
  qtyText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.text,
    minWidth: 24,
    textAlign: "center",
  },
  lineTotal: {
    fontSize: 15,
    fontWeight: "700",
    color: theme.text,
  },
});
}
