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
import type { Store } from "@repo/mobile-shared/api/types";

interface StoreSelectorProps {
  visible: boolean;
  onClose: () => void;
}

export function StoreSelector({ visible, onClose }: StoreSelectorProps) {
  const activeStore = useTenantStore((s) => s.activeStore);
  const { data: stores, isLoading } = useStores();
  const switchStore = useSwitchStore();

  const handleSelect = (store: Store) => {
    switchStore(store);
    onClose();
  };

  const renderItem = ({ item }: { item: Store }) => {
    const isActive = activeStore?.id === item.id;
    return (
      <TouchableOpacity
        style={[styles.storeRow, isActive && styles.storeRowActive]}
        onPress={() => handleSelect(item)}
        activeOpacity={0.7}
        accessibilityRole="button"
        accessibilityState={{ selected: isActive }}
      >
        <View style={styles.storeInfo}>
          <Text style={styles.storeName}>{item.name}</Text>
          <Text style={styles.storeSlug}>{item.slug}</Text>
        </View>
        {isActive && <Text style={styles.checkmark}>&#x2713;</Text>}
      </TouchableOpacity>
    );
  };

  return (
    <Modal visible={visible} transparent animationType="fade">
      <View style={styles.overlay}>
        <View style={styles.sheet}>
          <View style={styles.header}>
            <Text style={styles.title}>Select Store</Text>
            <TouchableOpacity onPress={onClose} accessibilityRole="button">
              <Text style={styles.closeText}>Done</Text>
            </TouchableOpacity>
          </View>
          {isLoading ? (
            <View style={styles.loading}>
              <ActivityIndicator size="small" color="#0E0E0C" />
            </View>
          ) : (
            <FlatList
              data={stores ?? []}
              renderItem={renderItem}
              keyExtractor={(item) => item.id}
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
    backgroundColor: "#FFFFFF",
    borderTopLeftRadius: 14,
    borderTopRightRadius: 14,
    maxHeight: "60%",
    paddingBottom: 34,
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 14,
    borderBottomWidth: 0.5,
    borderBottomColor: "#0E0E0C10",
  },
  title: {
    fontSize: 16,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  closeText: {
    fontSize: 15,
    fontWeight: "600",
    color: "#2D4A2B",
  },
  loading: {
    padding: 40,
    alignItems: "center",
  },
  listContent: {
    paddingVertical: 8,
  },
  storeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 16,
    paddingVertical: 14,
  },
  storeRowActive: {
    backgroundColor: "#F7F6F2",
  },
  storeInfo: {
    flex: 1,
  },
  storeName: {
    fontSize: 15,
    fontWeight: "600",
    color: "#0E0E0C",
    marginBottom: 2,
  },
  storeSlug: {
    fontSize: 12,
    color: "#0E0E0C",
    opacity: 0.4,
  },
  checkmark: {
    fontSize: 18,
    fontWeight: "700",
    color: "#2D4A2B",
    marginLeft: 8,
  },
  emptyText: {
    fontSize: 13,
    color: "#0E0E0C",
    opacity: 0.4,
    fontStyle: "italic",
    textAlign: "center",
    padding: 24,
  },
});
