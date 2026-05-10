import {
  ActivityIndicator,
  FlatList,
  RefreshControl,
  StyleSheet,
  TouchableOpacity,
  View,
} from "react-native";
import { Image } from "expo-image";
import { Stack, useRouter } from "expo-router";
import { ChevronLeft, Heart, Trash2 } from "lucide-react-native";
import { useRemoveFromWishlist, useWishlist } from "@/lib/hooks/use-wishlist";
import { Card, EmptyState, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/format";
import type { WishlistItem } from "@repo/mobile-shared/api/storefront-types";

export default function WishlistScreen() {
  const router = useRouter();
  const { data, isLoading, isRefetching, refetch } = useWishlist();
  const remove = useRemoveFromWishlist();
  const items = data?.items ?? [];

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <View style={styles.headerBar}>
        <TouchableOpacity
          onPress={() => router.back()}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Back"
          style={styles.backBtn}
        >
          <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
        </TouchableOpacity>
      </View>
      <PageHeader
        eyebrow="ACCOUNT"
        title="Wishlist"
        subtitle={items.length ? `${items.length} item${items.length === 1 ? "" : "s"}` : undefined}
      />
      <FlatList
        data={items}
        keyExtractor={(it) => it.product_id}
        renderItem={({ item }) => (
          <WishlistRow
            item={item}
            onPress={() => router.push(`/products/${item.handle}`)}
            onRemove={() => remove.mutate(item.product_id)}
          />
        )}
        ItemSeparatorComponent={() => <Hairline inset={88 + theme.spacing.lg * 2} />}
        contentContainerStyle={styles.list}
        refreshControl={
          <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
        }
        ListEmptyComponent={
          isLoading ? (
            <View style={styles.center}>
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : (
            <View style={styles.center}>
              <EmptyState
                icon={<Heart size={28} color={theme.colors.textTertiary} strokeWidth={1.5} />}
                title="Save what you love"
                message="Tap the heart on any product to save it for later."
              />
            </View>
          )
        }
      />
    </Screen>
  );
}

function WishlistRow({
  item,
  onPress,
  onRemove,
}: {
  item: WishlistItem;
  onPress: () => void;
  onRemove: () => void;
}) {
  return (
    <Card padding={0} style={{ marginVertical: theme.spacing.xs }}>
      <View style={styles.row}>
        <TouchableOpacity onPress={onPress} activeOpacity={0.6} style={styles.tapArea}>
          <View style={styles.thumb}>
            {item.image_url ? (
              <Image source={{ uri: item.image_url }} style={{ width: "100%", height: "100%" }} contentFit="cover" />
            ) : null}
          </View>
          <View style={{ flex: 1, gap: 2 }}>
            <Text preset="bodyEmphasis" color="text" numberOfLines={2}>
              {item.title}
            </Text>
            <Text preset="price" color="text">
              {formatMoney(item.price_amount, item.currency_code)}
            </Text>
            {item.stock_status !== "in_stock" ? (
              <Text preset="caption" color="warning">
                {item.stock_status === "out_of_stock" ? "Out of stock" : "Low stock"}
              </Text>
            ) : null}
          </View>
        </TouchableOpacity>
        <TouchableOpacity
          onPress={onRemove}
          hitSlop={12}
          accessibilityLabel={`Remove ${item.title} from wishlist`}
          style={styles.removeBtn}
        >
          <Trash2 size={16} color={theme.colors.danger} strokeWidth={1.75} />
        </TouchableOpacity>
      </View>
    </Card>
  );
}

const styles = StyleSheet.create({
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  list: { paddingHorizontal: theme.spacing.lg, paddingBottom: theme.spacing.huge },
  row: { flexDirection: "row", alignItems: "center", padding: theme.spacing.md },
  tapArea: { flex: 1, flexDirection: "row", alignItems: "center", gap: theme.spacing.md },
  thumb: {
    width: 88,
    height: 88,
    borderRadius: theme.radii.md,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
  removeBtn: { padding: theme.spacing.sm, marginLeft: theme.spacing.sm },
  center: { paddingVertical: theme.spacing.huge },
});
