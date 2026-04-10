import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MediaAltTextInput } from "./MediaAltTextInput";

describe("MediaAltTextInput", () => {
  it("commits value on blur", () => {
    const onCommit = vi.fn();
    render(<MediaAltTextInput value="" onCommit={onCommit} label="Alt text" />);
    const input = screen.getByLabelText(/alt text/i);
    fireEvent.change(input, { target: { value: "A red dress" } });
    fireEvent.blur(input);
    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onCommit).toHaveBeenCalledWith("A red dress");
  });

  it("does not call onCommit on every keystroke", () => {
    const onCommit = vi.fn();
    render(<MediaAltTextInput value="" onCommit={onCommit} label="Alt text" />);
    const input = screen.getByLabelText(/alt text/i);
    fireEvent.change(input, { target: { value: "A" } });
    fireEvent.change(input, { target: { value: "A r" } });
    fireEvent.change(input, { target: { value: "A red" } });
    expect(onCommit).not.toHaveBeenCalled();
  });
});
