import {
  forwardRef,
  useImperativeHandle,
  useRef,
  useState,
  type ComponentType,
  type ReactNode,
} from "react";
import { View, Pressable, StyleSheet, type StyleProp, type ViewStyle } from "react-native";
import { BottomSheetModal, BottomSheetScrollView } from "@gorhom/bottom-sheet";
import Animated, {
  FadeIn,
  FadeOut,
  LinearTransition,
  useReducedMotion,
} from "react-native-reanimated";
import { AlertTriangle, X } from "lucide-react-native";
import { Text, FieldInput } from "@/components/ui";
import { theme } from "@/lib/theme";
import { DISCLOSURE_EASING, DISCLOSURE_EXIT_DURATION } from "./disclosure-motion";
import type { UpdateProductOptionBody } from "@repo/mobile-shared/api/products";

/**
 * Turns raw sheet state into the request body — or `null` if the axis isn't
 * valid yet. Trims the name; trims + dedupes the values (first occurrence
 * wins, order preserved). Extracted as a pure function because the sheet
 * itself renders through a portal (gorhom's `BottomSheetModal`), which isn't
 * practical to mount in this project's jest setup — this is what gets
 * pinned by tests instead of the portal.
 */
export function buildOptionSubmission(
  name: string,
  values: string[],
): UpdateProductOptionBody | null {
  const trimmedName = name.trim();
  const seen = new Set<string>();
  const dedupedValues: string[] = [];
  for (const raw of values) {
    const value = raw.trim();
    if (value === "" || seen.has(value)) continue;
    seen.add(value);
    dedupedValues.push(value);
  }
  if (trimmedName === "" || dedupedValues.length === 0) return null;
  return { name: trimmedName, values: dedupedValues };
}

/**
 * @gorhom/bottom-sheet ships its own copy of @types/react, whose `ReactNode`
 * includes `bigint`; this project's does not, so its components trip TS2786
 * ("cannot be used as a JSX component"). Re-type it through this project's
 * React to the props we actually pass — runtime is unaffected. (The sibling
 * `BottomSheetView` was dodged entirely via a plain `View`; this scroll
 * wrapper has no plain-View equivalent, hence the cast.)
 */
const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{
  contentContainerStyle?: StyleProp<ViewStyle>;
  keyboardShouldPersistTaps?: "always" | "never" | "handled";
  children?: ReactNode;
}>;

export interface OptionBuilderSheetHandle {
  present: () => void;
}

interface OptionBuilderSheetProps {
  /** Called once with a valid axis; the sheet dismisses itself right after. */
  onSubmit: (option: UpdateProductOptionBody) => void;
}

export const OptionBuilderSheet = forwardRef<OptionBuilderSheetHandle, OptionBuilderSheetProps>(
  function OptionBuilderSheet({ onSubmit }, ref) {
    const modalRef = useRef<BottomSheetModal>(null);
    const [name, setName] = useState("");
    const [values, setValues] = useState<string[]>([]);
    const [draft, setDraft] = useState("");
    const reduceMotion = useReducedMotion();

    useImperativeHandle(ref, () => ({
      present: () => {
        // Every open starts from a clean axis — a stale name/values set from
        // a previous cancelled attempt must never leak into the next one.
        setName("");
        setValues([]);
        setDraft("");
        modalRef.current?.present();
      },
    }));

    const addChip = () => {
      const value = draft.trim();
      if (value === "" || values.includes(value)) return;
      setValues((prev) => [...prev, value]);
      setDraft("");
    };

    const removeChip = (value: string) => {
      setValues((prev) => prev.filter((v) => v !== value));
    };

    const canSubmit = name.trim() !== "" && values.length > 0;

    const handleConfirm = () => {
      const option = buildOptionSubmission(name, values);
      if (!option) return;
      onSubmit(option);
      modalRef.current?.dismiss();
    };

    return (
      <BottomSheetModal ref={modalRef} snapPoints={["60%"]} enableDynamicSizing={false}>
        <ScrollBody contentContainerStyle={styles.root} keyboardShouldPersistTaps="handled">
          <Text preset="h3" color="text">
            New option
          </Text>

          <FieldInput
            label="Name"
            value={name}
            onChangeText={setName}
            placeholder="Size, Colour…"
            accessibilityLabel="Option name"
            autoFocus
          />

          <View style={styles.valuesBlock}>
            <Text preset="caption" color="textTertiary">
              Values
            </Text>
            {values.length > 0 ? (
              <View style={styles.chips}>
                {values.map((value) => (
                  <Animated.View
                    key={value}
                    testID={`option-chip-${value}`}
                    layout={reduceMotion ? undefined : LinearTransition.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)}
                    entering={
                      reduceMotion
                        ? undefined
                        : FadeIn.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)
                    }
                    exiting={
                      reduceMotion
                        ? undefined
                        : FadeOut.duration(DISCLOSURE_EXIT_DURATION).easing(DISCLOSURE_EASING)
                    }
                    style={styles.chip}
                  >
                    <Text preset="caption" color="text">
                      {value}
                    </Text>
                    <Pressable
                      onPress={() => removeChip(value)}
                      accessibilityRole="button"
                      accessibilityLabel={`Remove ${value}`}
                      hitSlop={8}
                    >
                      <X size={12} color={theme.colors.textTertiary} strokeWidth={2.5} />
                    </Pressable>
                  </Animated.View>
                ))}
              </View>
            ) : null}
            <FieldInput
              value={draft}
              onChangeText={setDraft}
              onSubmitEditing={addChip}
              placeholder="Add a value…"
              accessibilityLabel="Add a value"
              returnKeyType="done"
            />
          </View>

          <View style={styles.consequence}>
            <AlertTriangle size={16} color={theme.colors.warning} strokeWidth={2} />
            <Text preset="caption" color="textSecondary" style={styles.consequenceText}>
              Adding an option creates a variation for each value. Your current price and stock
              stay on the first one; the new variations start empty — fill them in below.
            </Text>
          </View>

          <View style={styles.footer}>
            <Pressable
              style={styles.cancelButton}
              onPress={() => modalRef.current?.dismiss()}
              accessibilityRole="button"
              accessibilityLabel="Cancel"
            >
              <Text preset="bodyEmphasis" color="textSecondary">
                Cancel
              </Text>
            </Pressable>
            <Pressable
              style={[styles.confirmButton, !canSubmit && styles.confirmButtonDisabled]}
              onPress={handleConfirm}
              disabled={!canSubmit}
              accessibilityRole="button"
              accessibilityLabel="Add option"
              accessibilityState={{ disabled: !canSubmit }}
            >
              <Text preset="bodyEmphasis" color="inverse">
                Add option
              </Text>
            </Pressable>
          </View>
        </ScrollBody>
      </BottomSheetModal>
    );
  },
);

const styles = StyleSheet.create({
  root: { flexGrow: 1, padding: theme.spacing.lg, gap: theme.spacing.lg },
  valuesBlock: { gap: theme.spacing.xs },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.xs,
    paddingHorizontal: theme.spacing.sm,
    height: 32,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.elevated,
  },
  consequence: {
    flexDirection: "row",
    gap: theme.spacing.sm,
    padding: theme.spacing.md,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.surfaceAlt,
  },
  consequenceText: { flex: 1 },
  footer: {
    marginTop: "auto",
    flexDirection: "row",
    gap: theme.spacing.sm,
  },
  cancelButton: {
    flex: 1,
    height: 44,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  confirmButton: {
    flex: 1,
    height: 44,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.accent,
    alignItems: "center",
    justifyContent: "center",
  },
  confirmButtonDisabled: { opacity: 0.4 },
});
