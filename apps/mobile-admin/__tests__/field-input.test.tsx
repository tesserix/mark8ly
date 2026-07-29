import { render, fireEvent } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { FieldInput, FieldLabel } from "@/components/ui/FieldInput";
import { theme } from "@/lib/theme";
import { BODY_FONT_FAMILY } from "@/lib/fonts";

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

describe("FieldInput", () => {
  it("renders its label and value and reports changes", () => {
    const onChangeText = jest.fn();
    const { getByLabelText, getByText } = render(
      <FieldInput label="Title" value="Hat" onChangeText={onChangeText} accessibilityLabel="Title" />,
    );
    expect(getByText("Title")).toBeTruthy();
    fireEvent.changeText(getByLabelText("Title"), "Cap");
    expect(onChangeText).toHaveBeenCalledWith("Cap");
  });

  it("fires onBlur", () => {
    const onBlur = jest.fn();
    const { getByLabelText } = render(
      <FieldInput value="x" onChangeText={() => {}} onBlur={onBlur} accessibilityLabel="F" />,
    );
    fireEvent(getByLabelText("F"), "blur");
    expect(onBlur).toHaveBeenCalled();
  });

  it("FieldLabel renders caption text", () => {
    const { getByText } = render(<FieldLabel label="SKU" />);
    expect(getByText("SKU")).toBeTruthy();
  });

  it("renders input text at the app's 17pt body baseline in the SAME family <Text preset=\"body\"> renders through — not theme.fonts.sans, the OS system font (task 10)", () => {
    const { getByLabelText } = render(
      <FieldInput value="x" onChangeText={() => {}} accessibilityLabel="F" />,
    );
    const style = StyleSheet.flatten(getByLabelText("F").props.style);
    // BODY_FONT_FAMILY, not theme.fonts.sans — see lib/fonts.ts and
    // lib/fonts.test.ts for why these are not the same value.
    expect(style.fontFamily).toBe(BODY_FONT_FAMILY);
    expect(style.fontFamily).not.toBe(theme.fonts.sans);
    expect(style.fontSize).toBe(theme.text.body.fontSize);
    expect(style.lineHeight).toBe(theme.text.body.lineHeight);
  });

  it("keeps the box's 44pt minHeight — already on the baseline, not a defect this pass fixes (task 10)", () => {
    const { getByLabelText } = render(
      <FieldInput value="x" onChangeText={() => {}} accessibilityLabel="F" />,
    );
    const style = StyleSheet.flatten(getByLabelText("F").props.style);
    expect(style.minHeight).toBe(44);
  });
});
