import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { MediaCropDialog } from "./MediaCropDialog";

vi.mock("react-easy-crop", () => ({
  default: (props: {
    onCropComplete?: (
      area: { x: number; y: number; width: number; height: number },
      pixels: { x: number; y: number; width: number; height: number },
    ) => void;
  }): ReactElement => {
    setTimeout(() => {
      props.onCropComplete?.(
        { x: 0, y: 0, width: 100, height: 100 },
        { x: 10, y: 20, width: 200, height: 300 },
      );
    }, 0);
    return <div data-testid="mock-cropper" />;
  },
}));

vi.mock("./cropImage", () => ({
  cropToBlob: vi.fn(async () => new Blob(["x"], { type: "image/jpeg" })),
  loadImage: vi.fn(async () => ({ naturalWidth: 800, naturalHeight: 600 } as HTMLImageElement)),
}));

describe("MediaCropDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("emits blob + pixel box + rotation on Apply", async () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:test" onApply={onApply} onCancel={onCancel} />);

    // Wait for the mock to fire onCropComplete
    await waitFor(() => expect(screen.getByRole("button", { name: /apply/i })).not.toBeDisabled());

    fireEvent.click(screen.getByRole("button", { name: /apply/i }));

    await waitFor(() => expect(onApply).toHaveBeenCalledTimes(1));
    const [blob, box, rotation] = onApply.mock.calls[0];
    expect(blob).toBeInstanceOf(Blob);
    expect(box).toEqual({ x: 10, y: 20, width: 200, height: 300 });
    expect(rotation).toBe(0);
  });

  it("calls onCancel when Cancel clicked", async () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:test" onApply={onApply} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("calls onCancel on Escape key", async () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:test" onApply={onApply} onCancel={onCancel} />);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it("triggers Apply on Enter when pixelCrop is set", async () => {
    const onApply = vi.fn();
    const onCancel = vi.fn();
    render(<MediaCropDialog sourceUrl="blob:test" onApply={onApply} onCancel={onCancel} />);
    await waitFor(() => expect(screen.getByRole("button", { name: /apply/i })).not.toBeDisabled());
    fireEvent.keyDown(window, { key: "Enter" });
    await waitFor(() => expect(onApply).toHaveBeenCalledTimes(1));
  });
});
