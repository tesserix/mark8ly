import { useMemo, useState } from "react";
import {
  ActivityIndicator,
  ScrollView,
  StyleSheet,
  TouchableOpacity,
  View,
  Dimensions,
} from "react-native";
import { Image } from "expo-image";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { ChevronLeft, Heart, ShoppingCart, Star } from "lucide-react-native";
import { useProduct } from "@/lib/hooks/use-catalog";
import { useCartStore } from "@/lib/cart-store";
import {
  useAddToWishlist,
  useRemoveFromWishlist,
  useWishlistContains,
} from "@/lib/hooks/use-wishlist";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { Button, EmptyState, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";
import type {
  StorefrontProductDetail,
  StorefrontVariant,
} from "@repo/mobile-shared/api/storefront-types";

const { width: WINDOW_WIDTH } = Dimensions.get("window");

export default function ProductDetailScreen() {
  const router = useRouter();
  const { handle } = useLocalSearchParams<{ handle: string }>();
  const { data: product, isLoading } = useProduct(typeof handle === "string" ? handle : "");
  const addToCart = useCartStore((s) => s.add);
  const { user } = useAuth();
  const wishlistCheck = useWishlistContains(product?.id ?? "");
  const addToWishlist = useAddToWishlist();
  const removeFromWishlist = useRemoveFromWishlist();
  const inWishlist = wishlistCheck.data?.in_wishlist === true;

  const [selectedOptions, setSelectedOptions] = useState<Record<string, string>>({});

  const matchedVariant = useMemo<StorefrontVariant | null>(() => {
    if (!product) return null;
    if (product.variants.length === 0) return null;
    if (product.variants.length === 1) return product.variants[0]!;
    const v = product.variants.find((v) =>
      Object.entries(selectedOptions).every(([k, val]) => v.option_values[k] === val),
    );
    return v ?? null;
  }, [product, selectedOptions]);

  if (isLoading) {
    return (
      <Screen>
        <View style={styles.center}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  if (!product) {
    return (
      <Screen>
        <View style={styles.center}>
          <EmptyState
            title="Product not found"
            message="This product may have been removed."
            action={<Button label="Back to shop" onPress={() => router.replace("/shop")} />}
          />
        </View>
      </Screen>
    );
  }

  const onSale =
    product.compare_at_price &&
    Number(product.compare_at_price) > Number(product.price_amount);

  const variantPrice = matchedVariant
    ? formatMoney(matchedVariant.price_amount, product.currency_code)
    : formatMoney(product.price_amount, product.currency_code);

  const canAddToCart =
    product.variants.length === 0 ||
    matchedVariant !== null;

  const handleAdd = () => {
    if (!canAddToCart) return;
    const variant = matchedVariant ?? product.variants[0];
    addToCart({
      productId: product.id,
      variantId: variant?.id ?? product.id,
      handle: product.handle,
      title: product.title,
      variantTitle:
        variant && Object.keys(variant.option_values ?? {}).length
          ? Object.values(variant.option_values).join(" · ")
          : "",
      unitPriceAmount: variant?.price_amount ?? product.price_amount,
      currencyCode: product.currency_code,
      imageUrl: product.images?.[0]?.url ?? "",
    });
    router.push("/cart");
  };

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />

      <View style={styles.headerBar}>
        <TouchableOpacity
          onPress={() => router.back()}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Back"
          style={styles.headerBtn}
        >
          <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
        </TouchableOpacity>
        <View style={{ flexDirection: "row", gap: theme.spacing.sm }}>
          {user ? (
            <TouchableOpacity
              onPress={() => {
                if (!product?.id) return;
                if (inWishlist) {
                  removeFromWishlist.mutate(product.id);
                } else {
                  addToWishlist.mutate(product.id);
                }
              }}
              hitSlop={8}
              accessibilityRole="button"
              accessibilityLabel={inWishlist ? "Remove from wishlist" : "Add to wishlist"}
              style={styles.headerBtn}
            >
              <Heart
                size={20}
                color={inWishlist ? theme.colors.danger : theme.colors.text}
                fill={inWishlist ? theme.colors.danger : "transparent"}
                strokeWidth={1.75}
              />
            </TouchableOpacity>
          ) : null}
          <TouchableOpacity
            onPress={() => router.push("/cart")}
            hitSlop={8}
            accessibilityRole="link"
            accessibilityLabel="Open cart"
            style={styles.headerBtn}
          >
            <ShoppingCart size={20} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>
      </View>

      <ScrollView contentContainerStyle={styles.scroll}>
        <Gallery images={product.images} />

        <View style={styles.body}>
          {product.category_name ? (
            <Text preset="eyebrow" color="textTertiary">
              {product.category_name}
            </Text>
          ) : null}
          <Text preset="h1" color="text">
            {product.title}
          </Text>

          <View style={styles.priceRow}>
            <Text preset="h2" color="text">
              {variantPrice}
            </Text>
            {onSale ? (
              <Text preset="body" color="textTertiary" style={styles.compare}>
                {formatMoney(product.compare_at_price, product.currency_code)}
              </Text>
            ) : null}
          </View>

          {product.review_count > 0 ? (
            <View style={styles.ratingRow}>
              <Star size={14} color={theme.colors.text} fill={theme.colors.text} strokeWidth={0} />
              <Text preset="caption" color="text">
                {product.average_rating.toFixed(1)} · {product.review_count} review
                {product.review_count === 1 ? "" : "s"}
              </Text>
            </View>
          ) : null}

          <Hairline style={{ marginVertical: theme.spacing.lg }} />

          {product.options?.map((opt) => (
            <View key={opt.name} style={styles.optionGroup}>
              <Text preset="caption" color="textSecondary">
                {opt.name.toUpperCase()}
              </Text>
              <View style={styles.optionRow}>
                {opt.values.map((value) => {
                  const active = selectedOptions[opt.name] === value;
                  return (
                    <TouchableOpacity
                      key={value}
                      onPress={() =>
                        setSelectedOptions((p) => ({ ...p, [opt.name]: value }))
                      }
                      style={[styles.optionChip, active && styles.optionChipActive]}
                      activeOpacity={0.7}
                      accessibilityRole="button"
                      accessibilityState={{ selected: active }}
                      accessibilityLabel={`${opt.name}: ${value}`}
                    >
                      <Text preset="bodyEmphasis" color={active ? "inverse" : "text"}>
                        {value}
                      </Text>
                    </TouchableOpacity>
                  );
                })}
              </View>
            </View>
          ))}

          {product.description ? (
            <View style={{ marginTop: theme.spacing.lg, gap: theme.spacing.sm }}>
              <Text preset="eyebrow" color="textTertiary">
                DETAILS
              </Text>
              <Text preset="body" color="textSecondary">
                {product.description}
              </Text>
            </View>
          ) : null}
        </View>
      </ScrollView>

      <View style={styles.footer}>
        <Hairline />
        <View style={styles.footerInner}>
          <Button
            label={canAddToCart ? "Add to cart" : "Pick options"}
            onPress={handleAdd}
            disabled={!canAddToCart}
            fullWidth
          />
        </View>
      </View>
    </Screen>
  );
}

function Gallery({ images }: { images: StorefrontProductDetail["images"] }) {
  if (!images || images.length === 0) {
    return <View style={[styles.galleryItem, { backgroundColor: theme.colors.surfaceAlt }]} />;
  }
  return (
    <ScrollView
      horizontal
      pagingEnabled
      showsHorizontalScrollIndicator={false}
      style={styles.gallery}
    >
      {images.map((img) => (
        <Image
          key={img.id}
          source={{ uri: img.url }}
          style={styles.galleryItem}
          contentFit="cover"
          accessibilityIgnoresInvertColors
        />
      ))}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  headerBar: {
    position: "absolute",
    top: 0,
    left: 0,
    right: 0,
    flexDirection: "row",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.sm,
    zIndex: 10,
  },
  headerBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.elevated,
  },
  scroll: { paddingBottom: theme.spacing.huge * 2 },
  gallery: { width: WINDOW_WIDTH, height: WINDOW_WIDTH },
  galleryItem: { width: WINDOW_WIDTH, height: WINDOW_WIDTH },
  body: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.lg,
    gap: theme.spacing.xs,
  },
  priceRow: {
    flexDirection: "row",
    alignItems: "baseline",
    gap: theme.spacing.sm,
    marginTop: theme.spacing.xs,
  },
  compare: { textDecorationLine: "line-through" },
  ratingRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.xs,
    marginTop: theme.spacing.sm,
  },
  optionGroup: { marginBottom: theme.spacing.lg, gap: theme.spacing.sm },
  optionRow: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.sm },
  optionChip: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    backgroundColor: theme.colors.elevated,
    minHeight: 36,
    justifyContent: "center",
  },
  optionChipActive: {
    backgroundColor: theme.colors.primary,
    borderColor: theme.colors.primary,
  },
  footer: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    backgroundColor: theme.colors.background,
  },
  footerInner: {
    padding: theme.spacing.lg,
  },
});
