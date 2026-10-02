import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import type { PersonalisationField } from "@/lib/api/marketplace-api";
import { PersonalisationEditor } from "./PersonalisationEditor";
import { fieldSummary, keyFromLabel } from "./kinds";

function imageField(over: Partial<PersonalisationField> = {}): PersonalisationField {
  return {
    id: "f1",
    product_id: "p1",
    key: "your_photo",
    label: "Upload your photo",
    kind: "image",
    required: true,
    position: 10,
    max_images: 1,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    ...over,
  };
}

function selectField(over: Partial<PersonalisationField> = {}): PersonalisationField {
  return {
    id: "f2",
    product_id: "p1",
    key: "finish",
    label: "Finish",
    kind: "select",
    required: false,
    position: 20,
    options: [
      { id: "o1", value: "matte", label: "Matte", price_delta: "0", position: 0 },
      { id: "o2", value: "gloss", label: "Gloss", price_delta: "2.50", position: 10 },
    ],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    ...over,
  };
}

const ok = (field: PersonalisationField | null) =>
  Promise.resolve({ ok: true as const, field });

function noopDeps() {
  return {
    createField: vi.fn(() => ok(imageField())),
    updateField: vi.fn(() => ok(imageField())),
    deleteField: vi.fn(() => ok(null)),
    addOption: vi.fn(() => ok(selectField())),
    deleteOption: vi.fn(() => ok(selectField())),
  };
}

describe("PersonalisationEditor", () => {
  it("explains what the section is for when a product has no fields", () => {
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[]}
        {...noopDeps()}
      />,
    );
    // An empty state that only says "no fields" teaches nothing.
    expect(screen.getByText(/upload a photo/i)).toBeInTheDocument();
  });

  it("shows the key and a kind-aware summary for each field", () => {
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[imageField(), selectField()]}
        {...noopDeps()}
      />,
    );
    expect(screen.getByText("your_photo")).toBeInTheDocument();
    expect(screen.getByText(/one image/i)).toBeInTheDocument();
    expect(screen.getByText(/2 choices, 1 priced/i)).toBeInTheDocument();
  });

  it("toggling Required patches only that property", async () => {
    const deps = noopDeps();
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[imageField({ required: true })]}
        {...deps}
      />,
    );
    fireEvent.click(screen.getByRole("checkbox", { name: /required/i }));
    await waitFor(() =>
      expect(deps.updateField).toHaveBeenCalledWith("s1", "p1", "f1", {
        required: false,
      }),
    );
  });

  it("renders the server's field after a write, not the submitted draft", async () => {
    // The API applies defaults nobody typed. If the editor kept its own
    // optimistic row, a merchant would see blanks where the buyer will
    // see a real limit.
    const deps = noopDeps();
    deps.createField = vi.fn(() =>
      ok(imageField({ id: "f9", key: "art", label: "Your artwork", max_images: 3 })),
    );
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[]}
        {...deps}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /add personalisation/i }));
    fireEvent.change(screen.getByPlaceholderText(/upload your photo/i), {
      target: { value: "Your artwork" },
    });
    fireEvent.click(screen.getByRole("button", { name: /add field/i }));

    await waitFor(() => expect(screen.getByText("art")).toBeInTheDocument());
    expect(screen.getByText(/up to 3 images/i)).toBeInTheDocument();
  });

  it("surfaces a server validation message against the add form", async () => {
    const deps = noopDeps();
    deps.createField = vi.fn(() =>
      Promise.resolve({
        ok: false as const,
        error: { code: "validation_failed", message: "key is already in use" },
      }),
    );
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[]}
        {...deps}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /add personalisation/i }));
    fireEvent.change(screen.getByPlaceholderText(/upload your photo/i), {
      target: { value: "Your artwork" },
    });
    fireEvent.click(screen.getByRole("button", { name: /add field/i }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/already in use/i),
    );
    // The form stays open so the merchant can fix the key rather than
    // retyping the whole field.
    expect(screen.getByRole("button", { name: /add field/i })).toBeInTheDocument();
  });

  it("deleting a field removes its row", async () => {
    const deps = noopDeps();
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[imageField()]}
        {...deps}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /delete upload your photo/i }));
    await waitFor(() => expect(screen.queryByText("your_photo")).not.toBeInTheDocument());
  });

  it("refuses to remove the last choice of a select, before the server has to", () => {
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[
          selectField({
            options: [
              { id: "o1", value: "matte", label: "Matte", price_delta: "0", position: 0 },
            ],
          }),
        ]}
        {...noopDeps()}
      />,
    );
    const remove = screen.getByRole("button", { name: /^remove$/i });
    expect(remove).toBeDisabled();
    expect(remove).toHaveAttribute("title", expect.stringMatching(/at least one/i));
  });

  it("a select with two choices can lose one", async () => {
    const deps = noopDeps();
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[selectField()]}
        {...deps}
      />,
    );
    const removes = screen.getAllByRole("button", { name: /^remove$/i });
    expect(removes[0]).not.toBeDisabled();
    fireEvent.click(removes[0]!);
    await waitFor(() =>
      expect(deps.deleteOption).toHaveBeenCalledWith("s1", "p1", "f2", "o1"),
    );
  });

  it("cannot submit a choice field with no options", () => {
    render(
      <PersonalisationEditor
        storeId="s1"
        productId="p1"
        currencyCode="INR"
        initialFields={[]}
        {...noopDeps()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /add personalisation/i }));
    fireEvent.click(screen.getByRole("radio", { name: /choice/i }));
    fireEvent.change(screen.getByPlaceholderText(/upload your photo/i), {
      target: { value: "Finish" },
    });
    expect(screen.getByRole("button", { name: /add field/i })).toBeDisabled();
  });
});

describe("keyFromLabel", () => {
  it("produces keys the API's format check accepts", () => {
    // Must satisfy ^[a-z0-9]+(?:_[a-z0-9]+)*$ — the same CHECK is in
    // migration 000139, so a key this produces must never be rejected.
    const re = /^[a-z0-9]+(?:_[a-z0-9]+)*$/;
    for (const label of [
      "Upload your photo",
      "  Name to engrave  ",
      "Gift message!!",
      "Size / colour",
      "2026 edition",
      "---",
      "ü",
    ]) {
      expect(keyFromLabel(label)).toMatch(re);
    }
  });
});

describe("fieldSummary", () => {
  it("says free when no choice is priced", () => {
    expect(
      fieldSummary(
        selectField({
          options: [
            { id: "o1", value: "a", label: "A", price_delta: "0", position: 0 },
            { id: "o2", value: "b", label: "B", price_delta: "0", position: 1 },
          ],
        }),
        "INR",
      ),
    ).toMatch(/all free/i);
  });

  it("states a checkbox's price in the store's currency", () => {
    expect(
      fieldSummary(
        imageField({ kind: "checkbox", price_delta: "4.00", max_images: undefined }),
        "INR",
      ),
    ).toBe("adds INR 4.00");
  });
});
