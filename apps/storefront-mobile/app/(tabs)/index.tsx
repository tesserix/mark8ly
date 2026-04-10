import {
  View,
  Text,
  FlatList,
  StyleSheet,
  RefreshControl,
  ActivityIndicator,
  Pressable,
  Dimensions,
} from "react-native";
import { useRouter } from "expo-router";
import { useStoreBranding } from "@/lib/hooks/use-store-branding";
import { useCategories } from "@/lib/hooks/use-categories";
import { useProducts } from "@/lib/hooks/use-products";
import { HomeBanner } from "@/components/HomeBanner";
import { CategoryPills } from "@/components/CategoryPills";
import { ProductCard } from "@/components/ProductCard";
import { RecentlyViewed } from "@/components/RecentlyViewed";
import type { StorefrontProduct } from "@repo/mobile-shared/api/storefront-types";

const SCREEN_WIDTH = Dimensions.get("window").width;
const GRID_GAP = 12;
const GRID_PADDING = 16;
const CARD_WIDTH = (SCREEN_WIDTH - GRID_PADDING * 2 - GRID_GAP) / 2;

export default function HomeScreen() {
  const router = useRouter();
  const branding = useStoreBranding();
  const categories = useCategories();
  const featured = useProducts({ sort: "featured" });
  const newArrivals = useProducts({ sort: "newest" });

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

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!hasAnyContent && !hasBranding) {
    return (
      <View style={styles.centered}>
        <Text style={styles.emptyTitle}>Store coming soon</Text>
        <Text style={styles.emptySubtitle}>
          This store is being set up. Check back later.
        </Text>
        <Pressable
          style={styles.ctaButton}
          onPress={() => router.push("/(tabs)/browse")}
        >
          <Text style={styles.ctaText}>Browse all products</Text>
        </Pressable>
      </View>
    );
  }

  const renderFeaturedItem = ({ item }: { item: StorefrontProduct }) => (
    <View style={styles.featuredCardWrapper}>
      <ProductCard product={item} compact />
    </View>
  );

  const renderGridItem = ({ item }: { item: StorefrontProduct }) => (
    <View style={[styles.gridItem, { width: CARD_WIDTH }]}>
      <ProductCard product={item} />
    </View>
  );

  return (
    <FlatList
      data={newArrivalProducts}
      keyExtractor={(item) => `arrival-${item.id}`}
      numColumns={2}
      columnWrapperStyle={styles.gridRow}
      renderItem={renderGridItem}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={isRefreshing}
          onRefresh={handleRefresh}
          tintColor="#0E0E0C"
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
              <Text style={styles.sectionTitle}>Featured</Text>
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
              <Text style={styles.sectionTitle}>New arrivals</Text>
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
            color="#0E0E0C"
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
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    padding: 32,
  },
  content: {
    backgroundColor: "#F7F6F2",
    paddingBottom: 32,
  },
  section: {
    marginTop: 24,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: "#0E0E0C",
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
    color: "#0E0E0C",
    fontFamily: "SourceSerif4",
    marginBottom: 8,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
    marginBottom: 24,
    lineHeight: 20,
  },
  ctaButton: {
    backgroundColor: "#0E0E0C",
    paddingHorizontal: 24,
    paddingVertical: 14,
    borderRadius: 6,
  },
  ctaText: {
    color: "#F7F6F2",
    fontSize: 14,
    fontWeight: "600",
  },
});
