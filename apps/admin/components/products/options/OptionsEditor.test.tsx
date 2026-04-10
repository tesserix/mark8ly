import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { OptionsEditor, type OptionDraft } from "./OptionsEditor";

function sample(): OptionDraft[] {
  return [
    { id: "o1", name: "Size", values: [{ id: "v1", value: "S" }, { id: "v2", value: "M" }] },
    { id: "o2", name: "Color", values: [{ id: "v3", value: "Red" }] },
  ];
}

describe("OptionsEditor", () => {
  it("renders one row per option with name populated", () => {
    render(<OptionsEditor value={sample()} onChange={() => {}} />);
    expect(screen.getByDisplayValue("Size")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Color")).toBeInTheDocument();
  });

  it("Add option button appends empty option", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: /add option/i }));
    expect(onChange).toHaveBeenCalledWith([
      ...sample(),
      { name: "", values: [] },
    ]);
  });

  it("removing a row filters it out", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    const removeButtons = screen.getAllByRole("button", { name: /remove option/i });
    fireEvent.click(removeButtons[0]);
    expect(onChange).toHaveBeenCalledWith([sample()[1]]);
  });

  it("pressing Enter in add-value input appends a new value", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    const addInputs = screen.getAllByPlaceholderText(/add a value/i);
    fireEvent.change(addInputs[1], { target: { value: "Blue" } });
    fireEvent.keyDown(addInputs[1], { key: "Enter", code: "Enter" });
    expect(onChange).toHaveBeenCalledWith([
      sample()[0],
      { ...sample()[1], values: [...sample()[1].values, { value: "Blue" }] },
    ]);
  });

  it("clicking a value chip remove button removes that value", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    fireEvent.click(screen.getByRole("button", { name: /remove value S/i }));
    expect(onChange).toHaveBeenCalledWith([
      { ...sample()[0], values: [{ id: "v2", value: "M" }] },
      sample()[1],
    ]);
  });
});
