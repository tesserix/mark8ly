import { useCallback, useState } from "react";
import { View, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { useRouter } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useCreateTicket } from "@/lib/admin-api/ticket-actions";
import { BackHeader, FieldInput, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

const PRIORITIES = ["low", "normal", "high", "urgent"] as const;

function titleize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export default function NewTicketScreen() {
  const router = useRouter();
  const create = useCreateTicket();

  const [subject, setSubject] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<string>("normal");

  const canSubmit = subject.trim().length > 0 && description.trim().length > 0 && !create.isPending;

  const onSubmit = useCallback(() => {
    if (!canSubmit) return;
    create.mutate(
      { subject: subject.trim(), description: description.trim(), priority },
      {
        onSuccess: () => router.back(),
        onError: (err) =>
          Alert.alert(
            "Couldn't create ticket",
            err instanceof ApiError ? err.message : "Please try again.",
          ),
      },
    );
  }, [canSubmit, create, subject, description, priority, router]);

  return (
    <Screen>
      <BackHeader eyebrow="SETTINGS" title="New ticket" />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <FieldInput label="Subject" value={subject} onChangeText={setSubject} placeholder="What's it about?" />
        <FieldInput
          label="Description"
          value={description}
          onChangeText={setDescription}
          placeholder="Describe the issue"
          multiline
        />

        <Text preset="caption" color="textTertiary">
          Priority
        </Text>
        <View style={styles.chipRow}>
          {PRIORITIES.map((p) => (
            <Pressable
              key={p}
              style={[styles.chip, p === priority && styles.chipSelected]}
              onPress={() => setPriority(p)}
              accessibilityRole="button"
              accessibilityState={{ selected: p === priority }}
              accessibilityLabel={titleize(p)}
            >
              <Text preset="caption" color={p === priority ? "inverse" : "text"}>
                {titleize(p)}
              </Text>
            </Pressable>
          ))}
        </View>

        <Pressable
          style={[styles.btn, !canSubmit && styles.disabled]}
          onPress={onSubmit}
          disabled={!canSubmit}
          accessibilityRole="button"
          accessibilityLabel="Create ticket"
        >
          {create.isPending ? (
            <ActivityIndicator size="small" color={theme.colors.inverse} />
          ) : (
            <Text preset="bodyEmphasis" color="inverse">
              Create ticket
            </Text>
          )}
        </Pressable>
      </ScrollView>
    </Screen>
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
