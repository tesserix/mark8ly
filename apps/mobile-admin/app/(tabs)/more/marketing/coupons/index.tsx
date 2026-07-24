import { useCallback, useState } from "react";
import {
  View,
  FlatList,
  TouchableOpacity,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useCoupons } from "@/lib/hooks/use-coupons";
import { CouponRow } from "@/components/marketing/CouponRow";
import { BackHeader, EmptyState, Screen, SegmentedControl } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Coupon } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

type FilterKey = "all" | "active" | "scheduled" | "expired" | "disabled";

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: "all", label: "All" },
  { key: "active", label: "Active" },
  { key: "scheduled", label: "Scheduled" },
  { key: "expired", label: "Expired" },
  { key: "disabled", label: "Off" },
];

export default function CouponsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const currency = useTenantStore((s) => s.activeStore?.currency_code) || "AUD";
  const [filter, setFilter] = useState<FilterKey>("all");

  const {
    data,
    isLoading,
    isRefetching,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useCoupons(filter !== "all" ? { status: filter } : undefined);

  const coupons = data?.pages.flatMap((page) => page.data) ?? [];

  const handlePress = useCallback(
    (coupon: Coupon) => router.push(`/(tabs)/more/marketing/coupons/${coupon.id}`),
    [router],
  );

  const handleEndReached = useCallback(() => {
    if (hasNextPage && !isFetchingNextPage) fetchNextPage();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const renderItem = useCallback(
    ({ item }: { item: Coupon }) => (
      <CouponRow coupon={item} currency={currency} onPress={handlePress} />
    ),
    [currency, handlePress],
  );

  return (
    <Screen>
      <BackHeader
        eyebrow="MARKETING"
        title="Coupons"
        rightSlot={
          <TouchableOpacity
            onPress={() => router.push("/(tabs)/more/marketing/coupons/new")}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="New coupon"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        }
      />
      <View style={styles.filter}>
        <SegmentedControl<FilterKey> segments={FILTERS} value={filter} onChange={setFilter} />
      </View>

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && coupons.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
            title="Couldn't load coupons"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={coupons}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          onEndReached={handleEndReached}
          onEndReachedThreshold={0.5}
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
          }
          ListFooterComponent={
            isFetchingNextPage ? (
              <View style={styles.footer}>
                <ActivityIndicator size="small" color={theme.colors.text} />
              </View>
            ) : null
          }
          ListEmptyComponent={
            <EmptyState
              title="No coupons yet"
              message={
                filter !== "all"
                  ? "No coupons with this status."
                  : "Create a discount code to get started."
              }
            />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  filter: { paddingTop: theme.spacing.sm },
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  footer: { paddingVertical: theme.spacing.lg, alignItems: "center" },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
});
