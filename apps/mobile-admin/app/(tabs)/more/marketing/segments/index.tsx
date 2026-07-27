import { useCallback } from "react";
import {
  Platform,
  View,
  FlatList,
  Pressable,
  RefreshControl,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useRouter } from "expo-router";
import { Plus } from "lucide-react-native";
import { useSegments } from "@/lib/hooks/use-segments";
import { SegmentRow } from "@/components/marketing/SegmentRow";
import { BackHeader, EmptyState, Screen } from "@/components/ui";
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
          <Pressable
            onPress={() => router.push("/(tabs)/more/marketing/segments/new")}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="New segment"
            android_ripple={{ ...theme.press.rippleInk, borderless: true }}
            style={({ pressed }) =>
              pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null
            }
          >
            <Plus size={22} color={theme.colors.text} strokeWidth={1.75} />
          </Pressable>
        }
      />
      {isLoading && !isRefetching ? (
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      ) : isError && segments.length === 0 ? (
        <View style={styles.centered}>
          <EmptyState
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
});
