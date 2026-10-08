// The buyer's personalisation form, native (#969).
//
// One control per field the merchant defined. The form owns no rules of
// its own: validation lives in lib/personalisation.ts, the upload in
// lib/personalisation-upload.ts, the picker and HEIC conversion in
// lib/personalisation-picker.ts. This file is the glue and the pixels.
//
// Nothing here is a security boundary. Checkout revalidates every answer
// and reprices every delta from the catalog (#967).

import { useCallback, useState } from "react";
import {
  ActivityIndicator,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { Image } from "expo-image";
import { Check, ImagePlus } from "lucide-react-native";

import type { StorefrontPersonalisationField } from "@repo/mobile-shared/api/storefront-types";
import { useStorefrontApi } from "@/lib/api-client";
import { useCartStore } from "@/lib/cart-store";
import { formatMoney } from "@/lib/format";
import {
  isBelowMinimumResolution,
  type PersonalisationAnswers,
  type PersonalisationProblem,
} from "@/lib/personalisation";
import { pickPersonalisationImage } from "@/lib/personalisation-picker";
import {
  deletePersonalisationUpload,
  uploadPersonalisationImage,
} from "@/lib/personalisation-upload";
import { Hairline, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { MockupPreview } from "./MockupPreview";

export interface PersonalisationFormProps {
  productId: string;
  currencyCode: string;
  /** Already sorted — see sortFields. */
  fields: readonly StorefrontPersonalisationField[];
  answers: PersonalisationAnswers;
  onChange: (next: PersonalisationAnswers) => void;
  /** Problems to show, supplied by the parent after an add attempt. */
  problems?: readonly PersonalisationProblem[];
}

interface UploadState {
  busy: boolean;
  error?: string;
  /** From the picker's dimensions; drives the low-resolution warning only. */
  softWarning?: string;
  /** uploadId → local file URI, for the thumbnail and the mockup. */
  localUris: Record<string, string>;
}

const IDLE: UploadState = { busy: false, localUris: {} };

export function PersonalisationForm({
  productId,
  currencyCode,
  fields,
  answers,
  onChange,
  problems = [],
}: PersonalisationFormProps) {
  const api = useStorefrontApi();
  const ensureCartToken = useCartStore((s) => s.ensureCartToken);
  const [uploads, setUploads] = useState<Record<string, UploadState>>({});

  const stateFor = (fieldId: string) => uploads[fieldId] ?? IDLE;
  // Functional, so a patch can merge into the CURRENT state (another
  // photo's localUri) without the callback closing over `uploads`.
  const patchState = useCallback(
    (
      fieldId: string,
      patch:
        | Partial<UploadState>
        | ((prev: UploadState) => Partial<UploadState>),
    ) => {
      setUploads((u) => {
        const prev = u[fieldId] ?? IDLE;
        const next = typeof patch === "function" ? patch(prev) : patch;
        return { ...u, [fieldId]: { ...prev, ...next } };
      });
    },
    [],
  );

  const setAnswer = useCallback(
    (fieldId: string, patch: Partial<PersonalisationAnswers[string]>) => {
      onChange({ ...answers, [fieldId]: { ...answers[fieldId], ...patch } });
    },
    [answers, onChange],
  );

  const choosePhoto = useCallback(
    async (field: StorefrontPersonalisationField) => {
      patchState(field.id, {
        busy: true,
        error: undefined,
        softWarning: undefined,
      });
      try {
        const file = await pickPersonalisationImage();
        if (!file) {
          patchState(field.id, { busy: false });
          return;
        }
        // The token is minted here, on first need, so a buyer who never
        // uploads never has one — and one who does keeps it to checkout.
        const cartToken = ensureCartToken();
        const { uploadId, dimensions } = await uploadPersonalisationImage(api, {
          productId,
          fieldId: field.id,
          cartToken,
          file,
        });

        const existing = answers[field.id]?.uploadIds ?? [];
        const max = field.max_images ?? 1;
        // Replace rather than append when only one is allowed: a buyer
        // picking a second photo means "use this one instead", and
        // silently refusing would read as a broken picker.
        const nextIds =
          max === 1 ? [uploadId] : [...existing, uploadId].slice(0, max);
        if (max === 1 && existing[0] && existing[0] !== uploadId) {
          void deletePersonalisationUpload(api, existing[0], cartToken);
        }
        onChange({
          ...answers,
          [field.id]: { ...answers[field.id], uploadIds: nextIds },
        });
        patchState(field.id, (prev) => ({
          busy: false,
          localUris: { ...prev.localUris, [uploadId]: file.uri },
          softWarning: isBelowMinimumResolution(field, dimensions)
            ? `This photo is smaller than ${field.min_px}px, so it may look soft when printed. A larger photo will print better.`
            : undefined,
        }));
      } catch (err) {
        // A swallowed error here is a picker that "does nothing" — the
        // exact failure class the admin app exists to kill. Always shown.
        patchState(field.id, {
          busy: false,
          error:
            err instanceof Error
              ? err.message
              : "That photo could not be added.",
        });
      }
    },
    [answers, api, ensureCartToken, onChange, patchState, productId],
  );

  const removePhoto = useCallback(
    (field: StorefrontPersonalisationField, uploadId: string) => {
      const next = (answers[field.id]?.uploadIds ?? []).filter(
        (id) => id !== uploadId,
      );
      onChange({
        ...answers,
        [field.id]: { ...answers[field.id], uploadIds: next },
      });
      const token = useCartStore.getState().cartToken;
      if (token) void deletePersonalisationUpload(api, uploadId, token);
      patchState(field.id, { softWarning: undefined, error: undefined });
    },
    [answers, api, onChange, patchState],
  );

  if (fields.length === 0) return null;

  return (
    <View style={styles.section}>
      <Hairline style={{ marginBottom: theme.spacing.lg }} />
      <Text preset="h3" color="text">
        Make it yours
      </Text>

      {fields.map((field) => {
        const answer = answers[field.id] ?? {};
        const problem = problems.find((p) => p.fieldId === field.id)?.message;
        const state = stateFor(field.id);
        return (
          <View key={field.id} style={styles.field}>
            <View style={styles.labelRow}>
              <Text preset="bodyEmphasis" color="text">
                {field.label}
              </Text>
              <Text preset="caption" color="textTertiary">
                {field.required ? "Required" : "Optional"}
              </Text>
            </View>
            {field.help_text ? (
              <Text preset="caption" color="textSecondary">
                {field.help_text}
              </Text>
            ) : null}

            {field.kind === "image" ? (
              <View style={{ gap: theme.spacing.sm }}>
                {field.mockup_url && field.print_area ? (
                  <MockupPreview
                    mockupUrl={field.mockup_url}
                    printArea={field.print_area}
                    fieldLabel={field.label}
                    artworkUri={(answer.uploadIds ?? [])
                      .map((id) => state.localUris[id])
                      .find(Boolean)}
                  />
                ) : null}

                {(answer.uploadIds ?? []).map((id) => (
                  <View key={id} style={styles.thumbRow}>
                    <View style={styles.thumb}>
                      {state.localUris[id] ? (
                        <Image
                          source={{ uri: state.localUris[id] }}
                          style={StyleSheet.absoluteFill}
                          contentFit="cover"
                          accessibilityIgnoresInvertColors
                        />
                      ) : null}
                    </View>
                    <Text
                      preset="caption"
                      color="textSecondary"
                      style={{ flex: 1 }}
                    >
                      Photo added
                    </Text>
                    <TouchableOpacity
                      onPress={() => removePhoto(field, id)}
                      hitSlop={8}
                      accessibilityRole="button"
                      accessibilityLabel={`Remove photo for ${field.label}`}
                    >
                      <Text preset="caption" color="danger">
                        Remove
                      </Text>
                    </TouchableOpacity>
                  </View>
                ))}

                <TouchableOpacity
                  onPress={() => void choosePhoto(field)}
                  disabled={state.busy}
                  style={[styles.pickBtn, state.busy && styles.pickBtnBusy]}
                  activeOpacity={0.7}
                  accessibilityRole="button"
                  accessibilityState={{
                    busy: state.busy,
                    disabled: state.busy,
                  }}
                  accessibilityLabel={
                    (answer.uploadIds?.length ?? 0) > 0
                      ? `Replace photo for ${field.label}`
                      : `Choose photo for ${field.label}`
                  }
                >
                  {state.busy ? (
                    <ActivityIndicator size="small" color={theme.colors.text} />
                  ) : (
                    <ImagePlus
                      size={18}
                      color={theme.colors.text}
                      strokeWidth={1.75}
                    />
                  )}
                  <Text preset="bodyEmphasis" color="text">
                    {state.busy
                      ? "Uploading…"
                      : (answer.uploadIds?.length ?? 0) > 0
                        ? "Replace photo"
                        : "Choose photo"}
                  </Text>
                </TouchableOpacity>

                {state.softWarning ? (
                  // A warning, not an error: the buyer may know their photo
                  // is small and want it anyway.
                  <Text
                    preset="caption"
                    color="warning"
                    accessibilityRole="text"
                  >
                    {state.softWarning}
                  </Text>
                ) : null}
                {state.error ? (
                  <Text
                    preset="caption"
                    color="danger"
                    accessibilityRole="alert"
                  >
                    {state.error}
                  </Text>
                ) : null}
              </View>
            ) : null}

            {field.kind === "text" || field.kind === "textarea" ? (
              <TextInput
                value={answer.text ?? ""}
                onChangeText={(text) => setAnswer(field.id, { text })}
                maxLength={field.max_length}
                multiline={field.kind === "textarea"}
                numberOfLines={field.kind === "textarea" ? 3 : 1}
                style={[
                  styles.input,
                  field.kind === "textarea" && styles.textarea,
                  problem ? styles.inputInvalid : null,
                ]}
                placeholder={
                  field.kind === "textarea" ? "Your message" : undefined
                }
                placeholderTextColor={theme.colors.textTertiary}
                accessibilityLabel={field.label}
                accessibilityHint={field.help_text}
                autoCorrect={field.kind === "textarea"}
              />
            ) : null}
            {field.max_length !== undefined &&
            (field.kind === "text" || field.kind === "textarea") ? (
              <Text preset="caption" color="textTertiary" align="right">
                {(answer.text ?? "").length}/{field.max_length}
              </Text>
            ) : null}

            {field.kind === "select" ? (
              <View style={styles.chips}>
                {(field.options ?? []).map((opt) => {
                  const active = answer.optionId === opt.id;
                  const delta = Number.parseFloat(opt.price_delta);
                  return (
                    <TouchableOpacity
                      key={opt.id}
                      onPress={() => setAnswer(field.id, { optionId: opt.id })}
                      style={[styles.chip, active && styles.chipActive]}
                      activeOpacity={0.7}
                      accessibilityRole="button"
                      accessibilityState={{ selected: active }}
                      accessibilityLabel={`${field.label}: ${opt.label}`}
                    >
                      <Text
                        preset="bodyEmphasis"
                        color={active ? "inverse" : "text"}
                      >
                        {opt.label}
                      </Text>
                      {delta > 0 ? (
                        <Text
                          preset="caption"
                          color={active ? "inverse" : "textSecondary"}
                        >
                          +{formatMoney(delta, currencyCode)}
                        </Text>
                      ) : null}
                    </TouchableOpacity>
                  );
                })}
              </View>
            ) : null}

            {field.kind === "checkbox" ? (
              <TouchableOpacity
                onPress={() =>
                  setAnswer(field.id, { checked: !answer.checked })
                }
                style={styles.checkRow}
                activeOpacity={0.7}
                accessibilityRole="checkbox"
                accessibilityState={{ checked: answer.checked === true }}
                accessibilityLabel={field.label}
              >
                <View
                  style={[styles.checkBox, answer.checked && styles.checkBoxOn]}
                >
                  {answer.checked ? (
                    <Check
                      size={14}
                      color={theme.colors.inverse}
                      strokeWidth={2.5}
                    />
                  ) : null}
                </View>
                <Text preset="body" color="text" style={{ flex: 1 }}>
                  {field.price_delta && Number.parseFloat(field.price_delta) > 0
                    ? `Add for ${formatMoney(field.price_delta, currencyCode)}`
                    : "Yes"}
                </Text>
              </TouchableOpacity>
            ) : null}

            {problem ? (
              <Text preset="caption" color="danger" accessibilityRole="alert">
                {problem}
              </Text>
            ) : null}
          </View>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  section: { marginTop: theme.spacing.lg, gap: theme.spacing.lg },
  field: { gap: theme.spacing.sm },
  labelRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "baseline",
  },
  input: {
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.md,
    minHeight: theme.touchTarget,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.elevated,
  },
  textarea: { minHeight: 88, textAlignVertical: "top" },
  inputInvalid: { borderColor: theme.colors.danger, borderWidth: 1 },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.sm },
  chip: {
    paddingVertical: theme.spacing.sm,
    paddingHorizontal: theme.spacing.lg,
    borderRadius: theme.radii.pill,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    backgroundColor: theme.colors.elevated,
    minHeight: 36,
    justifyContent: "center",
    alignItems: "center",
    flexDirection: "row",
    gap: theme.spacing.xs,
  },
  chipActive: {
    backgroundColor: theme.colors.primary,
    borderColor: theme.colors.primary,
  },
  checkRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
    minHeight: theme.touchTarget,
  },
  checkBox: {
    width: 22,
    height: 22,
    borderRadius: theme.radii.sm,
    borderWidth: 1,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: theme.colors.elevated,
  },
  checkBoxOn: {
    backgroundColor: theme.colors.primary,
    borderColor: theme.colors.primary,
  },
  pickBtn: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing.sm,
    minHeight: theme.touchTarget,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.elevated,
  },
  pickBtnBusy: { opacity: 0.6 },
  thumbRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
  thumb: {
    width: 56,
    height: 56,
    borderRadius: theme.radii.md,
    overflow: "hidden",
    backgroundColor: theme.colors.surfaceAlt,
  },
});
