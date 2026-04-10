import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import {
  View,
  Text,
  ScrollView,
  StyleSheet,
  Pressable,
  ActivityIndicator,
  FlatList,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useProductByHandle, useProducts } from "@/lib/hooks/use-products";
import { useCartStore } from "@/stores/cart-store";
import { useRecentlyViewedStore } from "@/stores/recently-viewed-store";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { ProductGallery } from "@/components/ProductGallery";
import { VariantSelector } from "@/components/VariantSelector";
import { StarRating } from "@/components/StarRating";
import { ReviewCard } from "@/components/ReviewCard";
import { NotifyMeButton } from "@/components/NotifyMeButton";
import { WishlistButton } from "@/components/WishlistButton";
import { ProductCard } from "@/components/ProductCard";
import type {
  StorefrontVariant,
  StorefrontProduct,
} from "@repo/mobile-shared/api/storefront-types";

export default function ProductScreen() {
  const { handle } = useLocalSearchParams<{ handle: string }>();
  const theme = useTheme();
  const { data: product, isLoading, error, refetch } = useProductByHandle(handle ?? "");
  const addItem = useCartStore((s) => s.addItem);
  const addRecentProduct = useRecentlyViewedStore((s) => s.addProduct);
  const reviewsRef = useRef<View>(null);
  const scrollRef = useRef<ScrollView>(null);

  const [selectedValues, setSelectedValues] = useState<Record<string, string>>({});

  const related = useProducts({
    category: product?.category_name,
  });
  const relatedProducts = useMemo(() => {
    const all = related.data?.pages.flatMap((p) => p.products) ?? [];
    return all.filter((p) => p.handle !== handle).slice(0, 10);
  }, [related.data, handle]);

  useEffect(() => {
    if (product) {
      addRecentProduct({
        handle: product.handle,
        title: product.title,
        priceAmount: parseFloat(product.price_amount),
        currencyCode: product.currency_code,
        imageUrl: product.images[0]?.url,
      });
    }
  }, [product, addRecentProduct]);

  useEffect(() => {
    if (product && product.options.length > 0 && Object.keys(selectedValues).length === 0) {
      const initial: Record<string, string> = {};
      for (const option of product.options) {
        initial[option.name] = option.values[0] ?? "";
      }
      setSelectedValues(initial);
    }
  }, [product, selectedValues]);

  const handleOptionSelect = useCallback(
    (optionName: string, value: string) => {
      setSelectedValues((prev) => ({ ...prev, [optionName]: value }));
    },
    [],
  );

  const selectedVariant: StorefrontVariant | undefined = useMemo(() => {
    if (!product) return undefined;
    return product.variants.find((v) =>
      Object.entries(selectedValues).every(
        ([key, val]) => v.option_values[key] === val,
      ),
    );
  }, [product, selectedValues]);

  const currentPrice = selectedVariant?.price_amount ?? product?.price_amount ?? "0";
  const currentCompareAt = selectedVariant?.compare_at_price ?? product?.compare_at_price ?? "0";
  const stockStatus = selectedVariant?.stock_status ?? product?.stock_status ?? "in_stock";
  const stockQty = selectedVariant?.stock_quantity ?? product?.stock_quantity ?? 0;
  const isOutOfStock = stockStatus === "out_of_stock";
  const isLowStock = stockStatus === "low_stock" || (stockQty > 0 && stockQty <= 5);
  const hasDiscount = parseFloat(currentCompareAt) > parseFloat(currentPrice);

  const [reviewsExpanded, setReviewsExpanded] = useState(true);
  const [descriptionExpanded, setDescriptionExpanded] = useState(false);

  const themed = useMemo(
    () => ({
      container: { backgroundColor: theme.background },
      centered: { backgroundColor: theme.background },
      title: { color: theme.text },
      price: { color: theme.text },
      comparePrice: { color: theme.textSecondary },
      stockIn: { color: theme.accent },
      accordionSection: { borderTopColor: theme.border },
      accordionTitle: { color: theme.text },
      accordionChevron: { color: theme.textSecondary },
      reviewAverage: { color: theme.text },
      noReviews: { color: theme.textSecondary },
      descriptionText: { color: theme.textSecondary },
      relatedSection: { borderTopColor: theme.border },
      relatedTitle: { color: theme.text },
      stickyBar: { backgroundColor: theme.elevated, borderTopColor: theme.border },
      addToCartButton: { backgroundColor: theme.primary },
      addToCartDisabled: { backgroundColor: theme.border },
      addToCartText: { color: theme.elevated },
      errorText: { color: theme.text },
      retryButton: { borderColor: theme.text },
      retryText: { color: theme.text },
    }),
    [theme],
  );

  const handleAddToCart = useCallback(async () => {
    if (!product || isOutOfStock) return;
    await haptics.addToCart();
    addItem({
      productId: product.id,
      variantId: selectedVariant?.id ?? product.id,
      handle: product.handle,
      title: product.title,
      priceAmount: parseFloat(currentPrice),
      currencyCode: product.currency_code,
      imageUrl: product.images[0]?.url,
    });
  }, [product, selectedVariant, currentPrice, isOutOfStock, addItem]);

  const scrollToReviews = useCallback(() => {
    reviewsRef.current?.measureLayout(
      scrollRef.current as unknown as View,
      (_x, y) => {
        scrollRef.current?.scrollTo({ y, animated: true });
      },
      () => {},
    );
  }, []);

  const renderRelatedItem = useCallback(
    ({ item }: { item: StorefrontProduct }) => (
      <View style={styles.relatedCardWrapper}>
        <ProductCard product={item} compact />
      </View>
    ),
    [],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (error || !product) {
    return (
      <View style={[styles.centered, themed.centered]}>
        <Text style={[styles.errorText, themed.errorText]}>Failed to load product</Text>
        <Pressable
          style={[styles.retryButton, themed.retryButton]}
          onPress={() => refetch()}
          accessibilityRole="button"
          accessibilityLabel="Retry loading product"
        >
          <Text style={[styles.retryText, themed.retryText]}>Retry</Text>
        </Pressable>
      </View>
    );
  }

  const mockReviews = product.review_count > 0
    ? Array.from({ length: Math.min(product.review_count, 3) }, (_, i) => ({
        id: `review-${i}`,
        rating: product.average_rating,
        title: "",
        body: "",
        date: new Date().toISOString(),
      }))
    : [];

  return (
    <View style={[styles.container, themed.container]}>
      <ScrollView
        ref={scrollRef}
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
      >
        <View style={styles.wishlistOverlay}>
          <WishlistButton productId={product.id} />
        </View>

        <ProductGallery images={product.images} />

        <View style={styles.infoSection}>
          <Text style={[styles.title, themed.title]}>{product.title}</Text>

          <View style={styles.priceRow}>
            <Text style={[styles.price, themed.price]}>
              {product.currency_code} {currentPrice}
            </Text>
            {hasDiscount && (
              <Text style={[styles.comparePrice, themed.comparePrice]}>
                {product.currency_code} {currentCompareAt}
              </Text>
            )}
          </View>

          {product.average_rating > 0 && (
            <Pressable
              style={styles.ratingRow}
              onPress={scrollToReviews}
              accessibilityRole="button"
              accessibilityLabel={`${product.average_rating} stars, ${product.review_count} reviews. Tap to scroll to reviews`}
            >
              <StarRating
                rating={product.average_rating}
                count={product.review_count}
                size={14}
              />
            </Pressable>
          )}

          <View style={styles.stockRow}>
            {isOutOfStock ? (
              <Text style={styles.stockOut}>Out of stock</Text>
            ) : isLowStock ? (
              <Text style={styles.stockLow}>
                Low stock — {stockQty} left
              </Text>
            ) : (
              <Text style={[styles.stockIn, themed.stockIn]}>In stock</Text>
            )}
          </View>
        </View>

        {product.options.length > 0 && (
          <View style={styles.section}>
            <VariantSelector
              options={product.options}
              variants={product.variants}
              selectedValues={selectedValues}
              onSelect={handleOptionSelect}
            />
          </View>
        )}

        {isOutOfStock && (
          <View style={styles.section}>
            <NotifyMeButton productId={product.id} />
          </View>
        )}

        <View ref={reviewsRef} style={[styles.accordionSection, themed.accordionSection]}>
          <Pressable
            style={styles.accordionHeader}
            onPress={() => setReviewsExpanded((prev) => !prev)}
            accessibilityRole="button"
            accessibilityLabel={`Reviews section, ${reviewsExpanded ? "collapse" : "expand"}`}
          >
            <Text style={[styles.accordionTitle, themed.accordionTitle]}>
              Reviews
              {product.review_count > 0
                ? ` (${product.review_count})`
                : ""}
            </Text>
            <Text style={[styles.accordionChevron, themed.accordionChevron]}>
              {reviewsExpanded ? "−" : "+"}
            </Text>
          </Pressable>
          {reviewsExpanded && (
            <View style={styles.accordionBody}>
              {product.average_rating > 0 && (
                <View style={styles.reviewSummary}>
                  <Text style={[styles.reviewAverage, themed.reviewAverage]}>
                    {product.average_rating.toFixed(1)}
                  </Text>
                  <StarRating
                    rating={product.average_rating}
                    count={product.review_count}
                    size={16}
                  />
                </View>
              )}
              {mockReviews.length > 0 ? (
                mockReviews.map((review) => (
                  <ReviewCard
                    key={review.id}
                    rating={review.rating}
                    title={review.title}
                    body={review.body}
                    date={review.date}
                  />
                ))
              ) : (
                <Text style={[styles.noReviews, themed.noReviews]}>No reviews yet</Text>
              )}
            </View>
          )}
        </View>

        <View style={[styles.accordionSection, themed.accordionSection]}>
          <Pressable
            style={styles.accordionHeader}
            onPress={() => setDescriptionExpanded((prev) => !prev)}
            accessibilityRole="button"
            accessibilityLabel={`Description section, ${descriptionExpanded ? "collapse" : "expand"}`}
          >
            <Text style={[styles.accordionTitle, themed.accordionTitle]}>Description</Text>
            <Text style={[styles.accordionChevron, themed.accordionChevron]}>
              {descriptionExpanded ? "−" : "+"}
            </Text>
          </Pressable>
          {descriptionExpanded && (
            <View style={styles.accordionBody}>
              <Text style={[styles.descriptionText, themed.descriptionText]}>
                {product.description || "No description available."}
              </Text>
            </View>
          )}
        </View>

        {relatedProducts.length > 0 && (
          <View style={[styles.relatedSection, themed.relatedSection]}>
            <Text style={[styles.relatedTitle, themed.relatedTitle]}>You may also like</Text>
            <FlatList
              data={relatedProducts}
              keyExtractor={(item) => `related-${item.id}`}
              renderItem={renderRelatedItem}
              horizontal
              showsHorizontalScrollIndicator={false}
              contentContainerStyle={styles.relatedList}
            />
          </View>
        )}
      </ScrollView>

      <View style={[styles.stickyBar, themed.stickyBar]}>
        <Pressable
          style={[
            styles.addToCartButton,
            themed.addToCartButton,
            isOutOfStock && [styles.addToCartDisabled, themed.addToCartDisabled],
          ]}
          onPress={handleAddToCart}
          disabled={isOutOfStock}
          accessibilityRole="button"
          accessibilityLabel={isOutOfStock ? "Out of stock" : "Add to cart"}
        >
          <Text style={[styles.addToCartText, themed.addToCartText]}>
            {isOutOfStock ? "Out of stock" : "Add to cart"}
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
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  scrollContent: {
    paddingBottom: 100,
  },
  wishlistOverlay: {
    position: "absolute",
    top: 12,
    right: 12,
    zIndex: 10,
  },
  infoSection: {
    padding: 16,
    gap: 8,
  },
  title: {
    fontSize: 22,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
    lineHeight: 28,
  },
  priceRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  price: {
    fontSize: 18,
    fontWeight: "600",
  },
  comparePrice: {
    fontSize: 14,
    textDecorationLine: "line-through",
  },
  ratingRow: {
    flexDirection: "row",
    alignItems: "center",
  },
  stockRow: {
    marginTop: 4,
  },
  stockIn: {
    fontSize: 13,
    fontWeight: "500",
  },
  stockLow: {
    fontSize: 13,
    color: "#B87333",
    fontWeight: "500",
  },
  stockOut: {
    fontSize: 13,
    color: "#8B2500",
    fontWeight: "500",
  },
  section: {
    paddingHorizontal: 16,
    paddingBottom: 16,
  },
  accordionSection: {
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  accordionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 16,
  },
  accordionTitle: {
    fontSize: 16,
    fontWeight: "600",
  },
  accordionChevron: {
    fontSize: 20,
    fontWeight: "300",
  },
  accordionBody: {
    paddingHorizontal: 16,
    paddingBottom: 16,
  },
  reviewSummary: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    marginBottom: 12,
  },
  reviewAverage: {
    fontSize: 28,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
  },
  noReviews: {
    fontSize: 14,
  },
  descriptionText: {
    fontSize: 14,
    lineHeight: 22,
  },
  relatedSection: {
    marginTop: 8,
    paddingTop: 16,
    borderTopWidth: StyleSheet.hairlineWidth,
  },
  relatedTitle: {
    fontSize: 18,
    fontWeight: "600",
    paddingHorizontal: 16,
    marginBottom: 12,
    fontFamily: "SourceSerif4",
  },
  relatedList: {
    paddingHorizontal: 16,
    paddingBottom: 16,
  },
  relatedCardWrapper: {
    marginRight: 12,
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
  },
  addToCartButton: {
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  addToCartDisabled: {
    opacity: 1,
  },
  addToCartText: {
    fontSize: 16,
    fontWeight: "600",
  },
  errorText: {
    fontSize: 16,
    marginBottom: 16,
  },
  retryButton: {
    paddingHorizontal: 24,
    paddingVertical: 12,
    borderRadius: 6,
    borderWidth: 1,
  },
  retryText: {
    fontSize: 14,
    fontWeight: "600",
  },
});
