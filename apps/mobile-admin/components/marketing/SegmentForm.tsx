import { useState, useCallback } from "react";
import { View, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { FieldInput, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export interface SegmentFormValues {
  name: string;
  description?: string;
  rules: string;
}

interface SegmentFormProps {
  initialName?: string;
  initialDescription?: string;
  initialRules?: string;
  submitLabel: string;
  isSubmitting?: boolean;
  onSubmit: (values: SegmentFormValues) => void;
}

const RULES_PLACEHOLDER = '[{"field":"total_spent","operator":"gt","value":100}]';

/** Shared create/edit form for a customer segment. `rules` is the backend's
 *  opaque JSON-array string — edited as raw text here (power-user surface). */
export function SegmentForm({
  initialName = "",
  initialDescription = "",
  initialRules = "",
  submitLabel,
  isSubmitting = false,
  onSubmit,
}: SegmentFormProps) {
  const [name, setName] = useState(initialName);
  const [description, setDescription] = useState(initialDescription);
  const [rules, setRules] = useState(initialRules);

  const canSubmit = name.trim().length > 0 && rules.trim().length > 0 && !isSubmitting;

  const submit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit({
      name: name.trim(),
      ...(description.trim() ? { description: description.trim() } : {}),
      rules: rules.trim(),
    });
  }, [canSubmit, name, description, rules, onSubmit]);

  return (
    <View style={styles.form}>
      <FieldInput label="Name" value={name} onChangeText={setName} placeholder="High spenders" />
      <FieldInput
        label="Description (optional)"
        value={description}
        onChangeText={setDescription}
        placeholder="What this segment is for"
        multiline
      />
      <FieldInput
        label="Rules (JSON)"
        value={rules}
        onChangeText={setRules}
        placeholder={RULES_PLACEHOLDER}
        autoCapitalize="none"
        autoCorrect={false}
        multiline
      />
      <Text preset="caption" color="textTertiary">
        Rules are a JSON array of conditions. Build complex segments visually on the web dashboard.
      </Text>
      <Pressable
        style={[styles.btn, !canSubmit && styles.disabled]}
        onPress={submit}
        disabled={!canSubmit}
        accessibilityRole="button"
        accessibilityLabel={submitLabel}
      >
        {isSubmitting ? (
          <ActivityIndicator size="small" color={theme.colors.inverse} />
        ) : (
          <Text preset="bodyEmphasis" color="inverse">
            {submitLabel}
          </Text>
        )}
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  form: { gap: theme.spacing.md },
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
