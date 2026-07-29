import { TextInput, View, StyleSheet, type TextInputProps } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textTertiary">
      {label}
    </Text>
  );
}

interface FieldInputProps extends TextInputProps {
  label?: string;
}

/**
 * The one text input for the product forms. Standardised on surfaceAlt so
 * create and edit can't drift again (they each used to redefine styles.input,
 * one on elevated and one on surfaceAlt).
 */
export function FieldInput({ label, style, multiline, ...rest }: FieldInputProps) {
  return (
    <View style={styles.wrap}>
      {label ? <FieldLabel label={label} /> : null}
      <TextInput
        style={[styles.input, multiline ? styles.multiline : null, style]}
        placeholderTextColor={theme.colors.textTertiary}
        multiline={multiline}
        textAlignVertical={multiline ? "top" : "auto"}
        {...rest}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: theme.spacing.xs },
  input: {
    minHeight: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.surfaceAlt,
    // Was absent — TextInput rendered at RN's default size in the system
    // font, silently skipping the `body` preset every other text surface in
    // the app goes through (see Text.tsx). TextInput can't take a NativeWind
    // preset className the way <Text> does, so this resolves to the same
    // real values `SearchField`'s input already anchors to `theme.text.body`.
    fontFamily: theme.fonts.sans,
    fontSize: theme.text.body.fontSize,
    lineHeight: theme.text.body.lineHeight,
  },
  multiline: { minHeight: 96, paddingTop: theme.spacing.sm },
});
