import { useCallback } from "react";
import {
  View,
  Text,
  Modal,
  TouchableOpacity,
  FlatList,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useStores, useSwitchStore } from "../lib/hooks/use-store";
import { theme } from "@/lib/theme";
import type { Store } from "@repo/mobile-shared/api/types";

interface StoreSelectorProps {
  visible: boolean;
  onClose: () => void;
}

export function StoreSelector({ visible, onClose }: StoreSelectorProps) {
  const activeStore = useTenantStore((s) => s.activeStore);
  const { data: stores, isLoading } = useStores();
  const switchStore = useSwitchStore();

  const handleSelect = useCallback((store: Store) => {
    switchStore(store);
    onClose();
  }, [switchStore, onClose]);

  const renderItem = useCallback(({ item }: { item: Store }) => {
    const isActive = activeStore?.id === item.id;
    return (
      <TouchableOpacity
        style={[styles.storeRow, isActive && styles.storeRowActive]}
        onPress={() => handleSelect(item)}
        activeOpacity={0.7}
        accessibilityRole="button"
        accessibilityState={{ selected: isActive }}
        accessibilityLabel={`${item.name}${isActive ? ", currently selected" : ""}`}
      >
        <View style={styles.storeInfo}>
          <Text style={styles.storeName}>{item.name}</Text>
          <Text style={styles.storeSlug}>{item.slug}</Text>
        </View>
        {isActive && <Text style={styles.checkmark}>&#x2713;</Text>}
      </TouchableOpacity>
    );
  }, [activeStore?.id, handleSelect]);

  const keyExtractor = useCallback((item: Store) => item.id, []);

  return (
    <Modal visible={visible} transparent animationType="fade">
      <View style={styles.overlay}>
        <View style={styles.sheet}>
          <View style={styles.header}>
            <Text style={styles.title}>Select Store</Text>
            <TouchableOpacity
              onPress={onClose}
              accessibilityRole="button"
              accessibilityLabel="Done selecting store"
            >
              <Text style={styles.closeText}>Done</Text>
            </TouchableOpacity>
          </View>
          {isLoading ? (
            <View style={styles.loading}>
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : (
            <FlatList
              data={stores ?? []}
              renderItem={renderItem}
              keyExtractor={keyExtractor}
              contentContainerStyle={styles.listContent}
              ListEmptyComponent={
                <Text style={styles.emptyText}>No stores available</Text>
              }
            />
          )}
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.4)",
    justifyContent: "flex-end",
  },
  sheet: {
    backgroundColor: theme.colors.elevated,
    borderTopLeftRadius: 14,
    borderTopRightRadius: 14,
    maxHeight: "60%",
    paddingBottom: 34,
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 14,
    borderBottomWidth: 0.5,
    borderBottomColor: `${theme.colors.text}10`,
  },
  title: {
    fontSize: 16,
    fontWeight: "700",
    color: theme.colors.text,
  },
  closeText: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.accent,
  },
  loading: {
    padding: 40,
    alignItems: "center",
  },
  listContent: {
    paddingVertical: theme.spacing.sm,
  },
  storeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: 14,
    minHeight: 48,
  },
  storeRowActive: {
    backgroundColor: theme.colors.background,
  },
  storeInfo: {
    flex: 1,
  },
  storeName: {
    fontSize: 15,
    fontWeight: "600",
    color: theme.colors.text,
    marginBottom: 2,
  },
  storeSlug: {
    fontSize: 12,
    color: theme.colors.text,
    opacity: 0.4,
  },
  checkmark: {
    fontSize: 18,
    fontWeight: "700",
    color: theme.colors.accent,
    marginLeft: theme.spacing.sm,
  },
  emptyText: {
    fontSize: 13,
    color: theme.colors.text,
    opacity: 0.4,
    fontStyle: "italic",
    textAlign: "center",
    padding: theme.spacing.xxl,
  },
});
