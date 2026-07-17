import { useCallback, useState } from "react";
import { View, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useCreateCampaign } from "@/lib/admin-api/campaign-actions";
import { useSegments } from "@/lib/hooks/use-segments";
import { BackHeader, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";

export default function NewCampaignScreen() {
  const router = useRouter();
  const create = useCreateCampaign();
  const { data: segmentsData } = useSegments();
  const segments = segmentsData?.data ?? [];
  const dockPad = useDockClearance();

  const [name, setName] = useState("");
  const [subject, setSubject] = useState("");
  const [content, setContent] = useState("");
  const [segmentId, setSegmentId] = useState<string | undefined>(undefined);

  const canSubmit = name.trim().length > 0 && !create.isPending;

  const onSubmit = useCallback(() => {
    if (!canSubmit) return;
    create.mutate(
      {
        name: name.trim(),
        type: "email",
        ...(subject.trim() ? { subject: subject.trim() } : {}),
        ...(content.trim() ? { content: content.trim() } : {}),
        ...(segmentId ? { segment_id: segmentId } : {}),
      },
      {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't create campaign",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [canSubmit, create, name, subject, content, segmentId, router]);

  return (
    <Screen>
      <BackHeader eyebrow="MARKETING" title="New campaign" />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]} keyboardShouldPersistTaps="handled">
        <FieldInput label="Name" value={name} onChangeText={setName} placeholder="Summer sale blast" />
        <FieldInput
          label="Subject (optional)"
          value={subject}
          onChangeText={setSubject}
          placeholder="Email subject line"
        />
        <FieldInput
          label="Content (optional)"
          value={content}
          onChangeText={setContent}
          placeholder="Email body"
          multiline
        />

        <Text preset="caption" color="textTertiary">
          Audience
        </Text>
        <View style={styles.chipRow}>
          <AudienceChip label="Everyone" selected={!segmentId} onPress={() => setSegmentId(undefined)} />
          {segments.map((seg) => (
            <AudienceChip
              key={seg.id}
              label={seg.name}
              selected={segmentId === seg.id}
              onPress={() => setSegmentId(seg.id)}
            />
          ))}
        </View>

        <Text preset="caption" color="textTertiary">
          Campaigns start as drafts. Link a coupon, schedule, and preview from the web dashboard, then
          send from here.
        </Text>

        <Pressable
          style={[styles.btn, !canSubmit && styles.disabled]}
          onPress={onSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel="Create campaign"
        >
          {create.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Create draft
            </Text>
          )}
        </Pressable>
      </ScrollView>
    </Screen>
  );
}

function AudienceChip({
  label,
  selected,
  onPress,
}: {
  label: string;
  selected: boolean;
  onPress: () => void;
}) {
  return (
    <Pressable
      style={[styles.chip, selected && styles.chipSelected]}
      onPress={onPress}
      accessibilityRole="button"
      accessibilityState={{ selected }}
      accessibilityLabel={label}
    >
      <Text preset="caption" color={selected ? "inverse" : "text"} numberOfLines={1}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  chipRow: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.md,
    maxWidth: "100%",
    minHeight: theme.touchTarget,
    justifyContent: "center",
  },
  chipSelected: { backgroundColor: theme.colors.text, borderColor: theme.colors.text },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});
