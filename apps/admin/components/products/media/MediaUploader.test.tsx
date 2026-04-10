import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { MediaUploader } from "./MediaUploader";

describe("MediaUploader", () => {
  it("calls onFiles when files are selected via the file input", () => {
    const onFiles = vi.fn();
    render(<MediaUploader onFiles={onFiles} />);
    const input = screen.getByLabelText(/drop images/i) as HTMLInputElement;
    const file1 = new File(["a"], "a.jpg", { type: "image/jpeg" });
    const file2 = new File(["b"], "b.jpg", { type: "image/jpeg" });
    fireEvent.change(input, { target: { files: [file1, file2] } });
    expect(onFiles).toHaveBeenCalledTimes(1);
    expect(onFiles.mock.calls[0][0]).toHaveLength(2);
  });

  it("renders progress items", () => {
    render(
      <MediaUploader
        onFiles={vi.fn()}
        progressItems={[
          { id: "1", filename: "a.jpg", percent: 50, status: "uploading" },
          { id: "2", filename: "b.jpg", percent: 100, status: "done" },
        ]}
      />,
    );
    expect(screen.getByText("a.jpg")).toBeInTheDocument();
    expect(screen.getByText("b.jpg")).toBeInTheDocument();
  });
});
