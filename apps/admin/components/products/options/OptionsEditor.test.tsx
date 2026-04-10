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
    fireEvent.click(removeButtons[0]!);
    expect(onChange).toHaveBeenCalledWith([sample()[1]]);
  });

  it("pressing Enter in add-value input appends a new value", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    const addInputs = screen.getAllByPlaceholderText(/add a value/i);
    const target = addInputs[1]!;
    fireEvent.change(target, { target: { value: "Blue" } });
    fireEvent.keyDown(target, { key: "Enter", code: "Enter" });
    const second = sample()[1]!;
    expect(onChange).toHaveBeenCalledWith([
      sample()[0],
      { ...second, values: [...second.values, { value: "Blue" }] },
    ]);
  });

  it("changing option name calls onChange with updated name", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    fireEvent.change(screen.getByDisplayValue("Size"), { target: { value: "Sizes" } });
    expect(onChange).toHaveBeenCalledWith([
      { ...sample()[0], name: "Sizes" },
      sample()[1],
    ]);
  });

  it("pressing Enter with empty/whitespace value is a no-op", () => {
    const onChange = vi.fn();
    render(<OptionsEditor value={sample()} onChange={onChange} />);
    const addInputs = screen.getAllByPlaceholderText(/add a value/i);
    fireEvent.change(addInputs[0]!, { target: { value: "   " } });
    fireEvent.keyDown(addInputs[0]!, { key: "Enter" });
    // Non-Enter keydown also covered — no effect
    fireEvent.keyDown(addInputs[0]!, { key: "a" });
    expect(onChange).not.toHaveBeenCalled();
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
