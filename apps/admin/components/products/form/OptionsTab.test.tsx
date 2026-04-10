import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { FormProvider, useForm } from "react-hook-form";
import { useEffect, type ReactElement } from "react";
import { OptionsTab } from "./OptionsTab";
import type { ProductFormValues } from "@/lib/validation/product-form";

interface HarnessProps {
  options?: unknown[];
  variantError?: string;
}

function Harness({ options, variantError }: HarnessProps): ReactElement {
  const methods = useForm<ProductFormValues>({
    defaultValues: {
      options: (options ?? []) as never,
    } as Partial<ProductFormValues> as ProductFormValues,
  });
  useEffect(() => {
    if (variantError) {
      methods.setError("variants", { type: "manual", message: variantError });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [variantError]);
  return (
    <FormProvider {...methods}>
      <OptionsTab />
    </FormProvider>
  );
}

describe("OptionsTab", () => {
  it("renders without variant error banner by default and writes to form on add", () => {
    render(<Harness options={[{ id: "o1", name: "Size", values: [] }]} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /add option/i }));
    // After add, OptionsEditor re-renders with 2 rows
    // (form state updates internally via setValue)
    expect(screen.getByDisplayValue("Size")).toBeInTheDocument();
  });

  it("shows variant error banner when form has variants error", () => {
    render(<Harness variantError="Cannot exceed 100 variants" />);
    expect(screen.getByRole("alert")).toHaveTextContent(/cannot exceed 100 variants/i);
  });
});
