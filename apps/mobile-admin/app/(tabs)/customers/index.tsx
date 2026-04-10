import { useState, useCallback, useEffect } from "react";
import {
  View,
  Text,
  TextInput,
  FlatList,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { useCustomers } from "../../../lib/hooks/use-customers";
import { CustomerRow } from "../../../components/CustomerRow";
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

  const handleCustomerPress = useCallback(
    (customer: Customer) => {
      router.push(`/(tabs)/customers/${customer.id}`);
    },
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Customer }) => (
      <CustomerRow customer={item} onPress={handleCustomerPress} />
    ),
    [handleCustomerPress],
  );

  const keyExtractor = useCallback((item: Customer) => item.id, []);

  return (
    <View style={styles.screen}>
      <View style={styles.searchContainer}>
        <TextInput
          style={styles.searchInput}
          placeholder="Search customers..."
          placeholderTextColor={`${theme.colors.text}50`}
          value={searchText}
          onChangeText={setSearchText}
          autoCapitalize="none"
          autoCorrect={false}
          returnKeyType="search"
          accessibilityLabel="Search customers"
        />
      </View>

      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="large" color={theme.colors.text} />
        </View>
      ) : (
        <FlatList
          data={data?.items ?? []}
          renderItem={renderItem}
          keyExtractor={keyExtractor}
          contentContainerStyle={styles.listContent}
          refreshControl={
            <RefreshControl
              refreshing={isRefetching}
              onRefresh={refetch}
              tintColor={theme.colors.text}
            />
          }
          ListEmptyComponent={
            <View style={styles.centered}>
              <Text style={styles.emptyTitle}>No customers found</Text>
              <Text style={styles.emptySubtitle}>
                {debouncedSearch
                  ? "Try a different search term"
                  : "Customers will appear here once they sign up"}
              </Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: theme.colors.background,
  },
  searchContainer: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.md,
    paddingBottom: theme.spacing.md,
  },
  searchInput: {
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radius,
    paddingHorizontal: 14,
    paddingVertical: 10,
    fontSize: 14,
    color: theme.colors.text,
    borderWidth: 0.5,
    borderColor: `${theme.colors.text}10`,
  },
  listContent: {
    paddingTop: theme.spacing.xs,
    paddingBottom: theme.spacing.xxl,
    flexGrow: 1,
  },
  centered: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    paddingTop: 80,
  },
  emptyTitle: {
    fontSize: 16,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: theme.spacing.xs,
  },
  emptySubtitle: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.5,
  },
});
