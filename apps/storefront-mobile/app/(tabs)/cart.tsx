import { useCallback, useMemo, useState } from "react";
import {
  View,
  Text,
  FlatList,
  Pressable,
  StyleSheet,
  Alert,
  ActivityIndicator,
  RefreshControl,
} from "react-native";
import { useRouter } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { useCartStore, type CartItem as CartItemType } from "@/stores/cart-store";
import { useCartStockCheck } from "@/lib/hooks/use-cart-stock-check";
import { CartItem } from "@/components/CartItem";

export default function CartScreen() {
  const router = useRouter();
  const theme = useTheme();
  const { user } = useAuth();
  const items = useCartStore((s) => s.items);
  const subtotal = useCartStore((s) => s.subtotal);
  const updateQty = useCartStore((s) => s.updateQty);
  const removeItem = useCartStore((s) => s.removeItem);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const { stockMap, hasOutOfStock, refetchAll } = useCartStockCheck(items);

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      separator: { backgroundColor: theme.border },
      subtotalLabel: { color: theme.text },
      subtotalValue: { color: theme.text },
      stickyBar: { backgroundColor: theme.elevated, borderTopColor: theme.border },
      checkoutButton: { backgroundColor: theme.primary },
      checkoutButtonDisabled: { backgroundColor: theme.border },
      checkoutButtonText: { color: theme.elevated },
      emptyContainer: { backgroundColor: theme.background },
      emptyIllustration: { backgroundColor: theme.elevated },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
      shopButton: { backgroundColor: theme.primary },
      shopButtonText: { color: theme.elevated },
      subtotalBorderTop: { borderTopColor: theme.border },
    }),
    [theme],
  );

  const handleRefresh = useCallback(async () => {
    setIsRefreshing(true);
    await haptics.pullToRefresh();
    refetchAll();
    setTimeout(() => setIsRefreshing(false), 800);
  }, [refetchAll]);

  const handleSaveForLater = useCallback(
    async (variantId: string) => {
      await haptics.removeFromCart();
      removeItem(variantId);
      Alert.alert("Saved for later", "This item has been saved for later.");
    },
    [removeItem],
  );

  const handleCheckout = useCallback(() => {
    if (hasOutOfStock) return;

    if (!user) {
      Alert.alert(
        "Sign in for faster checkout",
        "You can sign in for saved addresses and order history, or continue as a guest.",
        [
          {
            text: "Sign In",
            onPress: () => router.push("/account/login"),
          },
          {
            text: "Continue as Guest",
            onPress: () => router.push("/checkout/details"),
          },
        ],
      );
      return;
    }

    router.push("/checkout/details");
  }, [user, hasOutOfStock, router]);

  const handleStartShopping = useCallback(() => {
    router.navigate("/(tabs)/browse");
  }, [router]);

  const renderItem = useCallback(
    ({ item }: { item: CartItemType }) => (
      <CartItem
        variantId={item.variantId}
        title={item.title}
        imageUrl={item.imageUrl}
        priceAmount={item.priceAmount}
        currencyCode={item.currencyCode}
        qty={item.qty}
        stockInfo={stockMap.get(item.variantId)}
        onUpdateQty={updateQty}
        onRemove={removeItem}
        onSaveForLater={handleSaveForLater}
      />
    ),
    [stockMap, updateQty, removeItem, handleSaveForLater],
  );

  if (items.length === 0) {
    return (
      <View style={[styles.emptyContainer, themed.emptyContainer]}>
        <View style={[styles.emptyIllustration, themed.emptyIllustration]}>
          <Text style={styles.emptyIcon}>🛒</Text>
        </View>
        <Text style={[styles.emptyTitle, themed.emptyTitle]}>
          Your cart is empty
        </Text>
        <Text style={[styles.emptySubtitle, themed.emptySubtitle]}>
          Browse our collection and find something you love.
        </Text>
        <Pressable
          style={[styles.shopButton, themed.shopButton]}
          onPress={handleStartShopping}
          accessibilityRole="button"
          accessibilityLabel="Start shopping"
        >
          <Text style={[styles.shopButtonText, themed.shopButtonText]}>
            Start shopping
          </Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={[styles.container, themed.container]}>
      <FlatList
        data={items}
        keyExtractor={(item) => item.variantId}
        renderItem={renderItem}
        contentContainerStyle={styles.listContent}
        ItemSeparatorComponent={() => (
          <View style={[styles.separator, themed.separator]} />
        )}
        showsVerticalScrollIndicator={false}
        refreshControl={
          <RefreshControl
            refreshing={isRefreshing}
            onRefresh={handleRefresh}
            tintColor={theme.primary}
          />
        }
        ListFooterComponent={
          <View style={[styles.subtotalRow, themed.subtotalBorderTop]}>
            <Text style={[styles.subtotalLabel, themed.subtotalLabel]}>
              Subtotal
            </Text>
            <Text style={[styles.subtotalValue, themed.subtotalValue]}>
              {subtotal()}
            </Text>
          </View>
        }
      />

      <View style={[styles.stickyBar, themed.stickyBar]}>
        {hasOutOfStock && (
          <Text style={styles.stockWarning}>
            Some items are out of stock or have limited availability
          </Text>
        )}
        <Pressable
          style={[
            styles.checkoutButton,
            themed.checkoutButton,
            hasOutOfStock && [styles.checkoutButtonDisabled, themed.checkoutButtonDisabled],
          ]}
          onPress={handleCheckout}
          disabled={hasOutOfStock}
          accessibilityRole="button"
          accessibilityLabel="Proceed to checkout"
        >
          <Text style={[styles.checkoutButtonText, themed.checkoutButtonText]}>
            Checkout
          </Text>
        </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
  },
  listContent: {
    paddingBottom: 140,
  },
  separator: {
    height: StyleSheet.hairlineWidth,
  },
  subtotalRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    padding: 16,
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  subtotalLabel: {
    fontSize: 16,
    fontWeight: "600",
  },
  subtotalValue: {
    fontSize: 18,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
  },
  stickyBar: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 32,
    borderTopWidth: StyleSheet.hairlineWidth,
    gap: 8,
  },
  stockWarning: {
    fontSize: 12,
    color: "#8B2500",
    textAlign: "center",
  },
  checkoutButton: {
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  checkoutButtonDisabled: {
    opacity: 1,
  },
  checkoutButtonText: {
    fontSize: 16,
    fontWeight: "600",
  },
  emptyContainer: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
    gap: 12,
  },
  emptyIllustration: {
    width: 80,
    height: 80,
    borderRadius: 40,
    alignItems: "center",
    justifyContent: "center",
    marginBottom: 8,
  },
  emptyIcon: {
    fontSize: 32,
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
    lineHeight: 20,
  },
  shopButton: {
    marginTop: 8,
    paddingHorizontal: 32,
    paddingVertical: 14,
    borderRadius: 6,
  },
  shopButtonText: {
    fontSize: 15,
    fontWeight: "600",
  },
});
