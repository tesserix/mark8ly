import { useState, useCallback, useEffect } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useRouter } from "expo-router";
import { useCustomers } from "../../../lib/hooks/use-customers";
import { CustomerRow } from "../../../components/CustomerRow";
import {
  EmptyState,
  PageHeader,
  Screen,
  SearchField,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Customer } from "@repo/mobile-shared/api/types";

function useDebounce(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export default function CustomersScreen() {
  const router = useRouter();
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);

  const { data, isLoading, isRefetching, refetch } = useCustomers(
    debouncedSearch || undefined,
  );

  const handlePress = useCallback(
    (customer: Customer) => router.push(`/(tabs)/customers/${customer.id}`),
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Customer }) => <CustomerRow customer={item} onPress={handlePress} />,
    [handlePress],
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
        <FlatList
          data={data?.items ?? []}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={styles.list}
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
