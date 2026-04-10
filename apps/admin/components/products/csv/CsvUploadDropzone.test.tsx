import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

import { CsvUploadDropzone } from "./CsvUploadDropzone";

describe("CsvUploadDropzone", () => {
  it("renders the dropzone with prompt text", () => {
    render(<CsvUploadDropzone onFileSelected={vi.fn()} />);
    expect(screen.getByText(/drag & drop/i)).toBeInTheDocument();
    expect(screen.getByText(/50,000 row/i)).toBeInTheDocument();
  });

  it("calls onFileSelected when a file is chosen via input", () => {
    const onFileSelected = vi.fn();
    render(<CsvUploadDropzone onFileSelected={onFileSelected} />);

    const input = screen.getByTestId("csv-file-input") as HTMLInputElement;
    const file = new File(["title,price\nT,10"], "products.csv", {
      type: "text/csv",
    });

    fireEvent.change(input, { target: { files: [file] } });
    expect(onFileSelected).toHaveBeenCalledWith(file);
  });

  it("rejects non-CSV files", () => {
    const onFileSelected = vi.fn();
    render(<CsvUploadDropzone onFileSelected={onFileSelected} />);

    const input = screen.getByTestId("csv-file-input") as HTMLInputElement;
    const file = new File(["not csv"], "image.png", { type: "image/png" });

    fireEvent.change(input, { target: { files: [file] } });
    expect(onFileSelected).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("shows selected filename", () => {
    render(
      <CsvUploadDropzone
        onFileSelected={vi.fn()}
        selectedFileName="products.csv"
      />,
    );
    expect(screen.getByText("products.csv")).toBeInTheDocument();
  });
});
