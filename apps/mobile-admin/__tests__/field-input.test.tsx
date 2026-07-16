import { render, fireEvent } from "@testing-library/react-native";
import { FieldInput, FieldLabel } from "@/components/ui/FieldInput";

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
});
