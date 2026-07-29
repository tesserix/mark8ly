// `@/components/ui`'s barrel re-exports BackHeader/SearchField, which import
// lucide-react-native icons jest-expo's default transformIgnorePatterns
// doesn't transform. Same one-line Proxy mock the other `__tests__/` suites
// touching the barrel already use.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { render, fireEvent } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { SearchField } from "@/components/ui/SearchField";
import { theme } from "@/lib/theme";
import { BODY_FONT_FAMILY } from "@/lib/fonts";

describe("SearchField", () => {
  it("renders its value and reports changes", () => {
    const onChangeText = jest.fn();
    const { getByLabelText } = render(
      <SearchField value="" onChangeText={onChangeText} placeholder="Search" />,
    );
    fireEvent.changeText(getByLabelText("Search"), "hat");
    expect(onChangeText).toHaveBeenCalledWith("hat");
  });

  it("renders input text in the SAME family <Text preset=\"body\"> renders through — not theme.fonts.sans, the OS system font (task 10)", () => {
    const { getByLabelText } = render(
      <SearchField value="" onChangeText={() => {}} placeholder="Search" />,
    );
    const style = StyleSheet.flatten(getByLabelText("Search").props.style);
    // BODY_FONT_FAMILY, not theme.fonts.sans — see lib/fonts.ts and
    // lib/fonts.test.ts for why these are not the same value.
    expect(style.fontFamily).toBe(BODY_FONT_FAMILY);
    expect(style.fontFamily).not.toBe(theme.fonts.sans);
    expect(style.fontSize).toBe(theme.text.body.fontSize);
  });
});
