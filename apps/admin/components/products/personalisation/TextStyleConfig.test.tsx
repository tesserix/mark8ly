import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { TextStyleConfig } from "./TextStyleConfig";

describe("TextStyleConfig", () => {
  it("offers fonts in the order they are ticked, the first being the default", () => {
    const onSave = vi.fn();
    render(<TextStyleConfig fonts={[]} colours={[]} onSave={onSave} />);

    fireEvent.click(screen.getByRole("button", { name: /Script/ }));
    fireEvent.click(screen.getByRole("button", { name: /Classic serif/ }));
    fireEvent.click(screen.getByRole("button", { name: "Save fonts and colours" }));

    expect(onSave).toHaveBeenCalledWith({ text_fonts: ["script", "classic"], text_colours: [] });
  });

  it("adds a named colour with its swatch, upper-cased", () => {
    const onSave = vi.fn();
    render(<TextStyleConfig fonts={[]} colours={[]} onSave={onSave} />);

    fireEvent.change(screen.getByLabelText("Swatch colour"), { target: { value: "#c9a227" } });
    fireEvent.change(screen.getByLabelText("Colour name"), { target: { value: " Gold foil " } });
    fireEvent.click(screen.getByRole("button", { name: "+ Add colour" }));
    fireEvent.click(screen.getByRole("button", { name: "Save fonts and colours" }));

    expect(onSave).toHaveBeenCalledWith({
      text_fonts: [],
      text_colours: [{ label: "Gold foil", hex: "#C9A227" }],
    });
  });

  it("refuses a second colour with the same name, which checkout could not tell apart", () => {
    render(
      <TextStyleConfig fonts={[]} colours={[{ label: "White", hex: "#FFFFFF" }]} onSave={vi.fn()} />,
    );
    fireEvent.change(screen.getByLabelText("Colour name"), { target: { value: "white" } });

    expect(screen.getByRole("button", { name: "+ Add colour" })).toBeDisabled();
    expect(screen.getByText("That name is already used.")).toBeInTheDocument();
  });

  it("cannot save until something changed", () => {
    render(<TextStyleConfig fonts={["script"]} colours={[]} onSave={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Save fonts and colours" })).toBeDisabled();
  });
});
