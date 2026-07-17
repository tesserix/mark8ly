import { useCallback } from "react";
import { View, ScrollView, TouchableOpacity, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useSegment } from "@/lib/hooks/use-segments";
import { useUpdateSegment, useDeleteSegment } from "@/lib/admin-api/segment-actions";
import { SegmentForm, type SegmentFormValues } from "@/components/marketing/SegmentForm";
import { BackHeader, Hairline, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

export default function SegmentDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: segment, isLoading, error } = useSegment(id);
  const update = useUpdateSegment();
  const del = useDeleteSegment();
  const dockPad = useDockClearance();

  const onSave = useCallback(
    (values: SegmentFormValues) => {
      // PATCH requires name + rules; description defaults to "" when cleared.
      update.mutate(
        { id, body: { name: values.name, description: values.description ?? "", rules: values.rules } },
        {
          onSuccess: () => router.back(),
          onError: (err) =>
            Alert.alert(
              "Couldn't save segment",
              err instanceof ApiError ? err.message : "Please try again.",
            ),
        },
      );
    },
    [id, update, router],
  );

  const onDelete = useCallback(() => {
    Alert.alert("Delete segment", "This removes the segment. Campaigns using it lose their audience.", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: () =>
          del.mutate(id, {
            onSuccess: () => router.back(),
            onError: (err) =>
              Alert.alert(
                "Couldn't delete segment",
                err instanceof ApiError ? err.message : "Please try again.",
              ),
          }),
      },
    ]);
  }, [id, del, router]);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="SEGMENT" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load segment
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !segment) {
    return (
      <Screen>
        <BackHeader eyebrow="SEGMENT" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <BackHeader eyebrow="SEGMENT" title={segment.name} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]} keyboardShouldPersistTaps="handled">
        <Text preset="caption" color="textTertiary">
          {segment.member_count} {segment.member_count === 1 ? "member" : "members"}
        </Text>
        <SegmentForm
          initialName={segment.name}
          initialDescription={segment.description ?? ""}
          initialRules={segment.rules}
          submitLabel={update.isPending ? "Saving…" : "Save changes"}
          isSubmitting={update.isPending}
          onSubmit={onSave}
        />
        <Hairline />
        <TouchableOpacity
          style={[styles.deleteBtn, del.isPending && styles.disabled]}
          onPress={onDelete}
          disabled={del.isPending}
          accessibilityRole="button"
          accessibilityLabel="Delete segment"
        >
          <Text preset="bodyEmphasis" color="danger">
            {del.isPending ? "Deleting…" : "Delete segment"}
          </Text>
        </TouchableOpacity>
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  deleteBtn: {
    height: 48,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.danger,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.5 },
});
