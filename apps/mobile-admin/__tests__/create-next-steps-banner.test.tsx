jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { render, fireEvent } from "@testing-library/react-native";
import { CreateNextStepsBanner } from "@/components/products/CreateNextStepsBanner";

describe("CreateNextStepsBanner", () => {
  it("renders the product title and all three jump chips", () => {
    const { getByText } = render(
      <CreateNextStepsBanner title="Ceramic Mug" onJump={jest.fn()} onDismiss={jest.fn()} />,
    );
    expect(getByText(`Nice — 'Ceramic Mug' is live.`)).toBeTruthy();
    expect(getByText("Add photos")).toBeTruthy();
    expect(getByText("Add options")).toBeTruthy();
    expect(getByText("Review variants")).toBeTruthy();
  });

  it("calls onJump with 'photos' when the photos chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Add photos"));
    expect(onJump).toHaveBeenCalledWith("photos");
  });

  it("calls onJump with 'options' when the options chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Add options"));
    expect(onJump).toHaveBeenCalledWith("options");
  });

  it("calls onJump with 'variants' when the variants chip is tapped", () => {
    const onJump = jest.fn();
    const { getByText } = render(
      <CreateNextStepsBanner title="Mug" onJump={onJump} onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Review variants"));
    expect(onJump).toHaveBeenCalledWith("variants");
  });

  it("calls onDismiss when the dismiss button is tapped", () => {
    const onDismiss = jest.fn();
    const { getByLabelText } = render(
      <CreateNextStepsBanner title="Mug" onJump={jest.fn()} onDismiss={onDismiss} />,
    );
    fireEvent.press(getByLabelText("Dismiss"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });
});
