import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import Animated, { FadeIn, useReducedMotion } from "react-native-reanimated";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useCustomers } from "../../../lib/hooks/use-customers";
import { CustomerRow } from "../../../components/CustomerRow";
import {
  EmptyState,
  PageHeader,
  Screen,
  SearchField,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING } from "@/components/products/disclosure-motion";
import type { Customer } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export default function CustomersScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const reduceMotion = useReducedMotion();
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const currencyCode = useTenantStore((s) => s.activeStore?.currency_code);

  const { data, isLoading, isRefetching, refetch } = useCustomers(
    debouncedSearch || undefined,
  );

  const handlePress = useCallback(
    (customer: Customer) => router.push(`/(tabs)/customers/${customer.id}`),
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Customer }) => (
      <CustomerRow customer={item} onPress={handlePress} currencyCode={currencyCode} />
    ),
    [handlePress, currencyCode],
  );

  return (
    <Screen>
      <PageHeader eyebrow="CUSTOMERS" title="People" />
      <View style={styles.search}>
        <SearchField
          value={searchText}
          onChangeText={setSearchText}
          placeholder="Search customers…"
          accessibilityLabel="Search customers"
        />
      </View>

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : (
        <Animated.View
          testID="customers-list-wrap"
          style={styles.listWrap}
          entering={reduceMotion ? undefined : FadeIn.duration(180).easing(DISCLOSURE_EASING)}
        >
          <FlatList
            data={data?.data ?? []}
            renderItem={renderItem}
            keyExtractor={(item) => item.id}
            contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
            refreshControl={
              <RefreshControl
                refreshing={isRefetching}
                onRefresh={refetch}
                tintColor={theme.colors.text}
              />
            }
            ListEmptyComponent={
              <EmptyState
                title="No customers yet"
                message={
                  debouncedSearch
                    ? "Try a different search term."
                    : "Customers appear here once they sign up."
                }
              />
            }
          />
        </Animated.View>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  search: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xs,
    paddingBottom: theme.spacing.sm,
  },
  listWrap: {
    flex: 1,
  },
  list: {
    flexGrow: 1,
    paddingBottom: theme.spacing.huge,
  },
  centered: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
});
