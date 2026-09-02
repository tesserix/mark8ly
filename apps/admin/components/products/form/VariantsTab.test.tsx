import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";
import type { ReactElement } from "react";
import { VariantsTab } from "./VariantsTab";
import type { ProductFormValues } from "@/lib/validation/product-form";

interface HarnessProps {
  variants?: unknown[];
  onSnapshot?: (values: unknown) => void;
}

function Harness({ variants, onSnapshot }: HarnessProps): ReactElement {
  const methods = useForm<ProductFormValues>({
    defaultValues: {
      variants: (variants ?? []) as never,
      media: [] as never,
    } as Partial<ProductFormValues> as ProductFormValues,
  });
  if (onSnapshot) {
    (methods as unknown as { __expose: unknown }).__expose = methods;
  }
  return (
    <FormProvider {...methods}>
      <VariantsTab currencyCode="USD" />
      <button
        type="button"
        data-testid="snapshot"
        onClick={() => onSnapshot?.(methods.getValues("variants"))}
      >
        snap
      </button>
    </FormProvider>
  );
}

describe("VariantsTab", () => {
  it("shows empty state when no variants", () => {
    render(<Harness />);
    expect(screen.getByText(/Add options below to generate variants/i)).toBeInTheDocument();
  });

  const sampleVariants = [
    {
      key: "v1",
      price: "10.00",
      sku: "S1",
      stock: 0,
      weight: 0,
      optionValues: [{ optionName: "Size", value: "S" }],
    },
    {
      key: "v2",
      price: "10.00",
      sku: "S2",
      stock: 0,
      weight: 0,
      optionValues: [{ optionName: "Size", value: "M" }],
    },
  ];

  it("renders matrix and commits a per-row price patch on blur", () => {
    const snap = vi.fn();
    render(<Harness variants={sampleVariants} onSnapshot={snap} />);
    const priceInputs = screen.getAllByLabelText("Price");
    fireEvent.change(priceInputs[0]!, { target: { value: "12.50" } });
    fireEvent.blur(priceInputs[0]!);
    fireEvent.click(screen.getByTestId("snapshot"));
    const result = snap.mock.calls[0]![0] as Array<{ key: string; price: string }>;
    expect(result[0]!.price).toBe("12.50");
    expect(result[1]!.price).toBe("10.00");
  });

  it("bulk set prices updates every variant via handleBulk", () => {
    const snap = vi.fn();
    render(<Harness variants={sampleVariants} onSnapshot={snap} />);
    fireEvent.click(screen.getByRole("button", { name: /set all prices/i }));
    const bulkPrice = document.getElementById("bulk-price") as HTMLInputElement;
    fireEvent.change(bulkPrice, { target: { value: "99.99" } });
    fireEvent.click(screen.getByRole("button", { name: /^apply$/i }));
    fireEvent.click(screen.getByTestId("snapshot"));
    const result = snap.mock.calls[0]![0] as Array<{ price: string }>;
    expect(result.every((v) => v.price === "99.99")).toBe(true);
  });
});
