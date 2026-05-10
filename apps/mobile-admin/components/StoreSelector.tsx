import { useCallback } from "react";
import {
  View,
  Modal,
  TouchableOpacity,
  FlatList,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { Check, X } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { useStores, useSwitchStore } from "../lib/hooks/use-store";
import { Hairline, Text } from "@/components/ui";
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

  const handleSelect = useCallback(
    (store: Store) => {
      switchStore(store);
      onClose();
    },
    [switchStore, onClose],
  );

  const renderItem = useCallback(
    ({ item }: { item: Store }) => {
      const isActive = activeStore?.id === item.id;
      return (
        <TouchableOpacity
          style={styles.storeRow}
          onPress={() => handleSelect(item)}
          activeOpacity={0.6}
          accessibilityRole="button"
          accessibilityState={{ selected: isActive }}
          accessibilityLabel={`${item.name}${isActive ? ", currently selected" : ""}`}
        >
          <View style={styles.storeInfo}>
            <Text preset="bodyEmphasis" color="text">
              {item.name}
            </Text>
            <Text preset="caption" color="textTertiary">
              {item.slug}
            </Text>
          </View>
          {isActive ? (
            <Check size={18} color={theme.colors.accent} strokeWidth={2} />
          ) : null}
        </TouchableOpacity>
      );
    },
    [activeStore?.id, handleSelect],
  );

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={styles.overlay}>
        <View style={styles.sheet}>
          <View style={styles.handle} />
          <View style={styles.header}>
            <Text preset="h3" color="text">
              Select Store
            </Text>
            <TouchableOpacity
              onPress={onClose}
              hitSlop={12}
              accessibilityRole="button"
              accessibilityLabel="Close"
            >
              <X size={20} color={theme.colors.text} strokeWidth={1.75} />
            </TouchableOpacity>
          </View>
          <Hairline />
          {isLoading ? (
            <View style={styles.loading}>
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : (
            <FlatList
              data={stores ?? []}
              renderItem={renderItem}
              keyExtractor={(item) => item.id}
              ItemSeparatorComponent={() => <Hairline inset={theme.spacing.lg} />}
              ListEmptyComponent={
                <View style={styles.empty}>
                  <Text preset="caption" color="textTertiary" align="center">
                    No stores available.
                  </Text>
                </View>
              }
              contentContainerStyle={styles.list}
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
    backgroundColor: theme.colors.overlay,
    justifyContent: "flex-end",
  },
  sheet: {
    backgroundColor: theme.colors.elevated,
    borderTopLeftRadius: theme.radii.xl,
    borderTopRightRadius: theme.radii.xl,
    maxHeight: "70%",
    paddingBottom: 34,
  },
  handle: {
    width: 36,
    height: 4,
    borderRadius: 2,
    backgroundColor: theme.colors.border,
    alignSelf: "center",
    marginTop: theme.spacing.sm,
    marginBottom: theme.spacing.xs,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
  },
  loading: { padding: theme.spacing.huge, alignItems: "center" },
  list: { paddingVertical: theme.spacing.xs },
  storeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
  },
  storeInfo: { flex: 1, gap: 2 },
  empty: { padding: theme.spacing.huge },
});
