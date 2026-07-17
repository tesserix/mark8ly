import { useCallback } from "react";
import { ScrollView, Alert, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useCreateSegment } from "@/lib/admin-api/segment-actions";
import { SegmentForm, type SegmentFormValues } from "@/components/marketing/SegmentForm";
import { BackHeader, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";

export default function NewSegmentScreen() {
  const router = useRouter();
  const create = useCreateSegment();

  const onSubmit = useCallback(
    (values: SegmentFormValues) => {
      create.mutate(values, {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't create segment",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      });
    },
    [create, router],
  );

  return (
    <Screen>
      <BackHeader eyebrow="MARKETING" title="New segment" />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <SegmentForm submitLabel="Create segment" isSubmitting={create.isPending} onSubmit={onSubmit} />
      </ScrollView>
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, paddingBottom: theme.spacing.huge },
});
