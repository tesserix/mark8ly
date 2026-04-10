import { useMemo, useCallback } from "react";
import {
  View,
  Text,
  FlatList,
  StyleSheet,
  RefreshControl,
  ActivityIndicator,
  Pressable,
  useWindowDimensions,
} from "react-native";
import { useRouter } from "expo-router";
import { useTheme } from "@/lib/theme/theme-provider";
import { useStoreBranding } from "@/lib/hooks/use-store-branding";
import { useCategories } from "@/lib/hooks/use-categories";
import { useProducts } from "@/lib/hooks/use-products";
import { HomeBanner } from "@/components/HomeBanner";
import { CategoryPills } from "@/components/CategoryPills";
import { ProductCard } from "@/components/ProductCard";
import { RecentlyViewed } from "@/components/RecentlyViewed";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const GRID_GAP = 12;
const GRID_PADDING = 16;

export default function HomeScreen() {
  const router = useRouter();
  const theme = useTheme();
  const { width: screenWidth } = useWindowDimensions();
  const branding = useStoreBranding();
  const categories = useCategories();
  const featured = useProducts({ sort: "featured" });
  const newArrivals = useProducts({ sort: "newest" });

  const cardWidth = (screenWidth - GRID_PADDING * 2 - GRID_GAP) / 2;

  const isLoading =
    branding.isLoading || categories.isLoading || featured.isLoading;

  const isRefreshing =
    branding.isRefetching || categories.isRefetching || featured.isRefetching;

  const handleRefresh = () => {
    branding.refetch();
    categories.refetch();
    featured.refetch();
    newArrivals.refetch();
  };

  const featuredProducts =
    featured.data?.pages.flatMap((p) => p.products) ?? [];
  const newArrivalProducts =
    newArrivals.data?.pages.flatMap((p) => p.products) ?? [];

  const hasBranding = branding.data?.banner_url;
  const hasAnyContent =
    featuredProducts.length > 0 || newArrivalProducts.length > 0;

  const themedStyles = useMemo(
    () => ({
      centered: { backgroundColor: theme.background },
      content: { backgroundColor: theme.background },
      sectionTitle: { color: theme.text },
      emptyTitle: { color: theme.text },
      emptySubtitle: { color: theme.textSecondary },
      ctaButton: { backgroundColor: theme.primary },
      ctaText: { color: theme.background },
    }),
    [theme],
  );

  const renderFeaturedItem = useCallback(
    ({ item }: { item: StorefrontProduct }) => (
      <View style={styles.featuredCardWrapper}>
        <ProductCard product={item} compact />
      </View>
    ),
    [],
  );

  const renderGridItem = useCallback(
    ({ item }: { item: StorefrontProduct }) => (
      <View style={[styles.gridItem, { width: cardWidth }]}>
        <ProductCard product={item} />
      </View>
    ),
    [cardWidth],
  );

  if (isLoading) {
    return (
      <View style={[styles.centered, themedStyles.centered]}>
        <ActivityIndicator size="large" color={theme.primary} />
      </View>
    );
  }

  if (!hasAnyContent && !hasBranding) {
    return (
      <View style={[styles.centered, themedStyles.centered]}>
        <Text style={[styles.emptyTitle, themedStyles.emptyTitle]}>
          Store coming soon
        </Text>
        <Text style={[styles.emptySubtitle, themedStyles.emptySubtitle]}>
          This store is being set up. Check back later.
        </Text>
        <Pressable
          style={[styles.ctaButton, themedStyles.ctaButton]}
          onPress={() => router.push("/(tabs)/browse")}
          accessibilityRole="button"
          accessibilityLabel="Browse all products"
        >
          <Text style={[styles.ctaText, themedStyles.ctaText]}>
            Browse all products
          </Text>
        </Pressable>
      </View>
    );
  }

  return (
    <FlatList
      data={newArrivalProducts}
      keyExtractor={(item) => `arrival-${item.id}`}
      numColumns={2}
      columnWrapperStyle={styles.gridRow}
      renderItem={renderGridItem}
      contentContainerStyle={[styles.content, themedStyles.content]}
      refreshControl={
        <RefreshControl
          refreshing={isRefreshing}
          onRefresh={handleRefresh}
          tintColor={theme.primary}
        />
      }
      ListHeaderComponent={
        <>
          <HomeBanner
            imageUrl={branding.data?.banner_url}
            title={branding.data?.banner_title}
          />

          {categories.data && categories.data.length > 0 && (
            <CategoryPills categories={categories.data} />
          )}

          {featuredProducts.length > 0 && (
            <View style={styles.section}>
              <Text style={[styles.sectionTitle, themedStyles.sectionTitle]}>
                Featured
              </Text>
              <FlatList
                data={featuredProducts}
                keyExtractor={(item) => `feat-${item.id}`}
                renderItem={renderFeaturedItem}
                horizontal
                showsHorizontalScrollIndicator={false}
                contentContainerStyle={styles.horizontalList}
              />
            </View>
          )}

          <RecentlyViewed />

          {newArrivalProducts.length > 0 && (
            <View style={styles.section}>
              <Text style={[styles.sectionTitle, themedStyles.sectionTitle]}>
                New arrivals
              </Text>
            </View>
          )}
        </>
      }
      onEndReached={() => {
        if (newArrivals.hasNextPage && !newArrivals.isFetchingNextPage) {
          newArrivals.fetchNextPage();
        }
      }}
      onEndReachedThreshold={0.5}
      ListFooterComponent={
        newArrivals.isFetchingNextPage ? (
          <ActivityIndicator
            size="small"
            color={theme.primary}
            style={styles.footerLoader}
          />
        ) : null
      }
    />
  );
}

const styles = StyleSheet.create({
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  content: {
    paddingBottom: 32,
  },
  section: {
    marginTop: 24,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    paddingHorizontal: 16,
    marginBottom: 12,
    fontFamily: "SourceSerif4",
  },
  horizontalList: {
    paddingHorizontal: 16,
  },
  featuredCardWrapper: {
    marginRight: 12,
  },
  gridRow: {
    paddingHorizontal: GRID_PADDING,
    gap: GRID_GAP,
  },
  gridItem: {
    marginBottom: GRID_GAP,
  },
  footerLoader: {
    paddingVertical: 20,
  },
  emptyTitle: {
    fontSize: 22,
    fontWeight: "700",
    fontFamily: "SourceSerif4",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    textAlign: "center",
    marginBottom: 24,
    lineHeight: 20,
  },
  ctaButton: {
    paddingHorizontal: 24,
    paddingVertical: 14,
    borderRadius: 6,
  },
  ctaText: {
    fontSize: 14,
    fontWeight: "600",
  },
});
