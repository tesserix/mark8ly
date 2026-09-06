import { StyleSheet, View } from "react-native";
import { OtpInput } from "react-native-otp-entry";
import { theme } from "@/lib/theme";
import { BODY_FONT_FAMILY } from "@/lib/fonts";

/**
 * Digits in an emailed sign-in code.
 *
 * Not a display choice — auth-bff's `emailotp.codeDigits` is 6, and its
 * verifier REJECTS any input that is not exactly six digits before it even
 * looks at the value. The field this replaced allowed eight, so a merchant
 * could type a code the server would refuse on length alone and be told the
 * code was wrong.
 */
export const CODE_LENGTH = 6;

interface CodeInputProps {
  /**
   * Uncontrolled: the cells own what has been typed and report it upward.
   * Nothing in this flow needs to push a value back down — an expired attempt
   * sends the merchant to /login, not back to a cleared field — so there is no
   * `value` prop to keep in sync with a control that would ignore it.
   */
  onChangeText: (code: string) => void;
  /** Fired once the last cell is filled — wire this to submit. */
  onFilled?: (code: string) => void;
  disabled?: boolean;
  accessibilityLabel: string;
  /**
   * Where the merchant reads the code from, which decides the AutoFill
   * hints.
   *
   * "email" asks for iOS one-time-code AutoFill. "authenticator" must NOT:
   * `oneTimeCode` / `one-time-code` are defined for codes DELIVERED to the
   * device by mail or SMS, and an authenticator code is never delivered —
   * offering the hint produces a suggestion banner that can only ever be
   * empty or, worse, offer an unrelated code from a message.
   *
   * Defaults to "email" so the existing OTP screen is unchanged.
   */
  codeSource?: "email" | "authenticator";
}

/**
 * One cell per digit, the convention every merchant has already met in every
 * other app that emails a code.
 *
 * It replaced a single centred TextInput whose caret sat mid-field with the
 * placeholder pushed off to one side, and which gave no feedback on how many
 * digits were expected or entered.
 *
 * Built on `react-native-otp-entry` rather than hand-rolled: six separate
 * inputs break iOS one-time-code AutoFill, which is the whole point on a
 * screen reached from an email. The library keeps ONE hidden TextInput behind
 * the cells, so AutoFill, paste and the keyboard all behave normally — and it
 * is MIT, dependency-free and pure JS, so it needs no config plugin and no
 * prebuild.
 *
 * Styling is passed through `theme` rather than className: NativeWind cannot
 * reach inside the library's own views, and these are real token values, not
 * approximations of them.
 */
export function CodeInput({
  onChangeText,
  onFilled,
  disabled,
  accessibilityLabel,
  codeSource = "email",
}: CodeInputProps) {
  const emailed = codeSource === "email";
  return (
    <View style={styles.wrap}>
      <OtpInput
        numberOfDigits={CODE_LENGTH}
        type="numeric"
        onTextChange={onChangeText}
        onFilled={onFilled}
        blurOnFilled
        disabled={disabled}
        // Off: the code arrives by email, so the merchant leaves the app to
        // read it. Grabbing focus on mount raises the keyboard over the
        // heading for a screen they are about to background anyway.
        autoFocus={false}
        focusColor={theme.colors.accent}
        textInputProps={{
          accessibilityLabel,
          // Both halves of iOS/Android AutoFill for a DELIVERED code.
          // Without them the code banner never offers itself above the
          // keyboard. Switched off for an authenticator code, which is not
          // delivered to the device at all — see `codeSource`.
          textContentType: emailed ? "oneTimeCode" : "none",
          autoComplete: emailed ? "one-time-code" : "off",
        }}
        theme={{
          containerStyle: styles.container,
          pinCodeContainerStyle: styles.cell,
          focusedPinCodeContainerStyle: styles.cellFocused,
          filledPinCodeContainerStyle: styles.cellFilled,
          pinCodeTextStyle: styles.cellText,
        }}
      />
    </View>
  );
}

const CELL_HEIGHT = 56;

const styles = StyleSheet.create({
  wrap: { marginTop: theme.spacing.xxl },
  container: { gap: theme.spacing.sm },
  cell: {
    height: CELL_HEIGHT,
    // 1, not `theme.hairline` (0.5). The half-pixel value is for hairline
    // RULES between sections; every FIELD in this app — including the email
    // and password inputs one screen back — draws a 1px border, and at 0.5 on
    // white-on-Paper the empty cells read as six floating white blobs with no
    // edge. Measured on device before changing it.
    borderWidth: 1,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.elevated,
  },
  // Moss, the one accent — the same colour the focus ring uses everywhere
  // else in this app.
  cellFocused: { borderColor: theme.colors.accent },
  // A filled cell reads as settled: ink hairline, no accent competing with
  // the cell the caret is actually in.
  cellFilled: { borderColor: theme.colors.text },
  cellText: {
    fontFamily: BODY_FONT_FAMILY,
    fontSize: 24,
    color: theme.colors.text,
  },
});
