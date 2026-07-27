import { useCallback } from "react";
import { FlatList, StyleSheet, View } from "react-native";
import { ChevronRight, Store as StoreIcon } from "lucide-react-native";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import type { Store } from "@repo/mobile-shared/api/types";
import { Hairline, PageHeader, PressableRow, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

interface StorePickerProps {
  stores: Store[];
}

/**
 * Full-screen picker shown after login when the user belongs to more
 * than one store and has no remembered selection. Whichever store the
 * user taps becomes the active one (and is persisted as the device's
 * default). The user can switch later from Account → Store.
 *
 * This is the mobile equivalent of the admin web's `/pick-tenant`
 * page — same intent (let a multi-tenant user pick a workspace),
 * adapted to a native list view.
 */
export function StorePicker({ stores }: StorePickerProps) {
  const setActiveStore = useTenantStore((s) => s.setActiveStore);

  const handleSelect = useCallback(
    (store: Store) => {
      setActiveStore(store);
    },
    [setActiveStore],
  );

  const renderItem = useCallback(
    ({ item }: { item: Store }) => (
      <PressableRow
        onPress={() => handleSelect(item)}
        style={styles.row}
        accessibilityLabel={`Open ${item.name}`}
      >
        <View style={styles.iconWrap}>
          <StoreIcon size={18} color={theme.colors.text} strokeWidth={1.75} />
        </View>
        <View style={styles.textWrap}>
          <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
            {item.name}
          </Text>
          <Text preset="caption" color="textTertiary" numberOfLines={1}>
            {item.slug}
          </Text>
        </View>
        <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
      </PressableRow>
    ),
    [handleSelect],
  );

  return (
    <Screen>
      <PageHeader
        eyebrow="STORE ACCESS"
        title="Pick a store"
        subtitle="Your account belongs to more than one store. Choose the one you want to open — you can switch any time from Account."
      />
      <View style={styles.listWrap}>
        <FlatList
          data={stores}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          ItemSeparatorComponent={() => <Hairline inset={theme.spacing.huge} />}
          contentContainerStyle={styles.listContent}
        />
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  listWrap: {
    flex: 1,
    marginHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.sm,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.lg,
    overflow: "hidden",
  },
  listContent: { paddingVertical: theme.spacing.xs },
  // Pre-migration this row had no backgroundColor of its own (transparent),
  // letting the parent listWrap's elevated (white) surface show through.
  // PressableRow's base sets backgroundColor: theme.colors.background
  // (paper), which would otherwise paint a visible seam against listWrap —
  // match that surface explicitly instead of relying on transparency (same
  // fix as DashboardOrderRow).
  row: { backgroundColor: theme.colors.elevated },
  iconWrap: {
    width: 32,
    height: 32,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.surfaceAlt,
    alignItems: "center",
    justifyContent: "center",
  },
  textWrap: { flex: 1, gap: 2 },
});
