import {
  FlatList,
  RefreshControl,
  ScrollView,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { Image } from "expo-image";
import { useRouter } from "expo-router";
import { useBranding } from "@/lib/hooks/use-branding";
import { useProducts, useCategories } from "@/lib/hooks/use-catalog";
import { ProductCard } from "@/components/ProductCard";
import { Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { getMerchant } from "@/lib/merchant";

export default function HomeScreen() {
  const router = useRouter();
  const merchant = getMerchant();
  const branding = useBranding();
  const products = useProducts();
  const categories = useCategories();

  const storeName = branding.data?.store_name ?? merchant.shortName;
  const logo = branding.data?.logo_url;
  const banner = branding.data?.banner_url;
  const bannerTitle = branding.data?.banner_title;

  return (
    <Screen>
      <ScrollView
        contentContainerStyle={styles.scroll}
        refreshControl={
          <RefreshControl
            refreshing={products.isRefetching || branding.isRefetching}
            onRefresh={() => {
              branding.refetch();
              products.refetch();
              categories.refetch();
            }}
            tintColor={theme.colors.text}
          />
        }
      >
        <PageHeader
          eyebrow="WELCOME"
          title={storeName}
          rightSlot={
            logo ? (
              <Image
                source={{ uri: logo }}
                style={styles.logo}
                contentFit="contain"
                accessibilityIgnoresInvertColors
              />
            ) : null
          }
        />

        {banner ? (
          <View style={styles.banner}>
            <Image source={{ uri: banner }} style={styles.bannerImage} contentFit="cover" />
            {bannerTitle ? (
              <View style={styles.bannerOverlay}>
                <Text preset="display" color="inverse">
                  {bannerTitle}
                </Text>
              </View>
            ) : null}
          </View>
        ) : null}

        {categories.data?.items?.length ? (
          <View style={styles.section}>
            <View style={styles.sectionHeader}>
              <Text preset="eyebrow" color="textTertiary">
                SHOP BY CATEGORY
              </Text>
            </View>
            <ScrollView
              horizontal
              showsHorizontalScrollIndicator={false}
              contentContainerStyle={styles.chips}
            >
              {categories.data.items.map((c) => (
                <TouchableOpacity
                  key={c.id}
                  onPress={() => router.push(`/shop?category=${c.slug}`)}
                  style={styles.chip}
                  activeOpacity={0.7}
                  accessibilityRole="button"
                  accessibilityLabel={`Shop ${c.name}`}
                >
                  <Text preset="bodyEmphasis" color="text">
                    {c.name}
                  </Text>
                </TouchableOpacity>
              ))}
            </ScrollView>
          </View>
        ) : null}

        <View style={styles.section}>
          <View style={styles.sectionHeader}>
            <Text preset="eyebrow" color="textTertiary">
              FEATURED
            </Text>
            <TouchableOpacity
              onPress={() => router.push("/shop")}
              hitSlop={8}
              accessibilityRole="link"
              accessibilityLabel="See all products"
            >
              <Text preset="caption" color="accent">
                See all
              </Text>
            </TouchableOpacity>
          </View>

          <FlatList
            data={products.data?.items?.slice(0, 6) ?? []}
            keyExtractor={(p) => p.id}
            numColumns={2}
            scrollEnabled={false}
            columnWrapperStyle={styles.row}
            ItemSeparatorComponent={() => <View style={{ height: theme.spacing.lg }} />}
            renderItem={({ item }) => <ProductCard product={item} />}
            ListEmptyComponent={
              <Text preset="caption" color="textTertiary" align="center">
                {products.isLoading ? "Loading…" : "No products yet."}
              </Text>
            }
            contentContainerStyle={styles.gridContent}
          />
        </View>

        <Hairline style={{ marginVertical: theme.spacing.lg }} />

        <View style={styles.footer}>
          <Text preset="caption" color="textTertiary" align="center">
            Powered by Mark8ly
          </Text>
        </View>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  logo: { width: 56, height: 56, borderRadius: theme.radii.md },
  banner: {
    marginHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.sm,
    height: 180,
    borderRadius: theme.radii.lg,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
  bannerImage: { width: "100%", height: "100%" },
  bannerOverlay: {
    position: "absolute",
    inset: 0,
    backgroundColor: "rgba(14,14,12,0.25)",
    justifyContent: "flex-end",
    padding: theme.spacing.lg,
  },
  section: {
    paddingHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.xl,
    gap: theme.spacing.sm,
  },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  chips: { gap: theme.spacing.sm, paddingVertical: theme.spacing.xs },
  chip: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    minHeight: 40,
    justifyContent: "center",
  },
  gridContent: { gap: theme.spacing.lg, paddingTop: theme.spacing.sm },
  row: { gap: theme.spacing.lg },
  footer: { paddingVertical: theme.spacing.xl, alignItems: "center" },
});
