import { useCallback } from "react";
import {
  View,
  FlatList,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useSegments } from "@/lib/hooks/use-segments";
import { SegmentRow } from "@/components/marketing/SegmentRow";
import { BackHeader, EmptyState, IconButton, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Segment } from "@repo/mobile-shared/api/types";
import { useDockClearance } from "@/components/navigation/dock-metrics";

export default function SegmentsScreen() {
  const dockPad = useDockClearance();
  const router = useRouter();
  const { data, isLoading, isRefetching, isError, refetch } = useSegments();
  const segments = data?.data ?? [];

  const handlePress = useCallback(
    (segment: Segment) => router.push(`/(tabs)/more/marketing/segments/${segment.id}`),
    [router],
  );

  const renderItem = useCallback(
    ({ item }: { item: Segment }) => <SegmentRow segment={item} onPress={handlePress} />,
    [handlePress],
  );

  return (
    <Screen>
      <BackHeader
        eyebrow="MARKETING"
        title="Segments"
        rightSlot={
          <IconButton
            onPress={() => router.push("/(tabs)/more/marketing/segments/new")}
            accessibilityLabel="New segment"
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </IconButton>
        }
      />
      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && segments.length === 0 ? (
        <View style={styles.errorSlot}>
          <EmptyState
            align="left"
            title="Couldn't load segments"
            message="Something went wrong. Check your connection and try again."
            action={{ label: "Try again", onPress: () => { refetch(); } }}
          />
        </View>
      ) : (
        <FlatList
          data={segments}
          renderItem={renderItem}
          keyExtractor={(item) => item.id}
          contentContainerStyle={[styles.list, { paddingBottom: dockPad }]}
          refreshControl={
            <RefreshControl refreshing={isRefetching} onRefresh={refetch} tintColor={theme.colors.text} />
          }
          ListEmptyComponent={
            <EmptyState
              align="left"
              title="No segments yet"
              message="Group customers to target campaigns at the right audience."
            />
          }
        />
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  list: { flexGrow: 1, paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  // NOT `styles.centered` for the error state: that wrapper's
  // `alignItems: "center"` shrink-wraps and re-centres its child, silently
  // undoing `EmptyState align="left"`. Claims the remaining height only.
  errorSlot: { flex: 1 },
});
