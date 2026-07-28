// The no-name customer is the ONLY case that exposes any of this. The demo
// tenant's real customer has no first/last name, just
// `mahesh.sangawar@gmail.com` — and guest checkout and CSV-imported customers
// routinely arrive the same way. Every test below that uses a customer WITH a
// name would pass against the broken code; the named cases are here only to
// prove the fix did not delete the email from the screens that should show it.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { CustomerRow } from "@/components/CustomerRow";
import { Monogram } from "@/components/ui";
import { customerIdentity } from "@/lib/customer-identity";
import { theme } from "@/lib/theme";
import type { Customer } from "@repo/mobile-shared/api/types";

const NO_NAME_EMAIL = "mahesh.sangawar@gmail.com";

const NO_NAME: Customer = {
  id: "c-real",
  email: NO_NAME_EMAIL,
  tags: [],
  status: "active",
  marketing_opt_in: false,
  order_count: 0,
  total_spent: 0,
  created_at: "2026-05-06T00:00:00Z",
  updated_at: "2026-05-06T00:00:00Z",
};

const NAMED: Customer = {
  ...NO_NAME,
  id: "c-named",
  first_name: "Ada",
  last_name: "Lovelace",
  email: "ada@example.com",
};

describe("customerIdentity — the email is the name, or it is the subtitle, never both", () => {
  it("titles a no-name customer with their email and gives them NO subtitle", () => {
    const identity = customerIdentity(NO_NAME);
    expect(identity.title).toBe(NO_NAME_EMAIL);
    expect(identity.subtitle).toBeUndefined();
    expect(identity.titleIsEmail).toBe(true);
  });

  it("titles a named customer with the name and puts the email beneath", () => {
    const identity = customerIdentity(NAMED);
    expect(identity.title).toBe("Ada Lovelace");
    expect(identity.subtitle).toBe("ada@example.com");
    expect(identity.titleIsEmail).toBe(false);
  });

  it("matches the Dashboard queue's fallback rule (name || email)", () => {
    // lib/queue.ts's orderToQueueItem is `customer_name || customer_email`.
    expect(customerIdentity({ first_name: "Ada", email: "a@b.c" }).title).toBe("Ada");
    expect(customerIdentity({ last_name: "Lovelace", email: "a@b.c" }).title).toBe("Lovelace");
    expect(customerIdentity({ email: "a@b.c" }).title).toBe("a@b.c");
  });

  it("treats a whitespace-only name as no name at all", () => {
    // An imported customer can carry a space where a CSV column was empty.
    // Truthiness alone would title the row with an invisible string and drop
    // the email entirely — a row that states nobody.
    const identity = customerIdentity({ first_name: "  ", last_name: "", email: NO_NAME_EMAIL });
    expect(identity.title).toBe(NO_NAME_EMAIL);
    expect(identity.titleIsEmail).toBe(true);
  });
});

describe("CustomerRow — a no-name customer's email appears ONCE", () => {
  // THE guard for defect 1. The primary line already fell back to the email;
  // the secondary line rendered `customer.email` unconditionally, so this row
  // printed `mahesh.sangawar@gmail.com` twice, stacked. Restoring that
  // unconditional secondary line turns `toHaveLength(1)` into 2.
  it("renders the email exactly once, not stacked twice", () => {
    const { getAllByText } = render(
      <CustomerRow customer={NO_NAME} onPress={jest.fn()} currencyCode="AUD" />,
    );
    expect(getAllByText(NO_NAME_EMAIL)).toHaveLength(1);
  });

  it("speaks the email once to VoiceOver too", () => {
    const { getByTestId } = render(
      <CustomerRow customer={NO_NAME} onPress={jest.fn()} currencyCode="AUD" />,
    );
    const label: string = getByTestId("customer-row-c-real").props.accessibilityLabel;
    expect(label.split(NO_NAME_EMAIL)).toHaveLength(2); // i.e. exactly one occurrence
  });

  it("still shows BOTH the name and the email for a customer who has one", () => {
    const { getByText } = render(
      <CustomerRow customer={NAMED} onPress={jest.fn()} currencyCode="AUD" />,
    );
    expect(getByText("Ada Lovelace")).toBeTruthy();
    expect(getByText("ada@example.com")).toBeTruthy();
  });
});

describe("Monogram — one leading-art shape across every screen", () => {
  // THE guard for defect 3. The customers list and detail drew circles
  // (`borderRadius: size / 2`) while the Dashboard queue and `Thumb` drew
  // rounded squares. Reverting either call site to a circle fails here.
  it("is a rounded square at Thumb's radius, at every size", () => {
    for (const size of [40, theme.thumb.list, 72]) {
      const { getByTestId } = render(<Monogram label="Ada" size={size} testID="m" />);
      const style = StyleSheet.flatten(getByTestId("m").props.style);
      expect(style.borderRadius).toBe(theme.radii.md);
      expect(style.borderRadius).not.toBe(size / 2);
      expect(style.width).toBe(size);
      expect(style.height).toBe(size);
    }
  });

  it("defaults to Thumb's 60pt list slot so a photo row and a no-photo row match", () => {
    const { getByTestId } = render(<Monogram label="Ada" testID="m" />);
    const style = StyleSheet.flatten(getByTestId("m").props.style);
    expect(style.width).toBe(theme.thumb.list);
    expect(style.backgroundColor).toBe(theme.colors.sink);
    expect(style.borderColor).toBe(theme.colors.textTertiary);
  });

  it("pins the initial's font scale so a fixed tile can't clip it at raised text sizes", () => {
    const { getByText } = render(<Monogram label="Ada" size={40} testID="m" />);
    expect(getByText("A").props.maxFontSizeMultiplier).toBe(1);
  });

  it("takes the initial from whatever the identity title is — including an email", () => {
    const { getByText } = render(
      <Monogram label={customerIdentity(NO_NAME).title} size={40} testID="m" />,
    );
    expect(getByText("M")).toBeTruthy();
  });
});

describe("CustomerRow — the list row's monogram is the shared tile", () => {
  it("draws a rounded square, not the circle it used to", () => {
    const { getByTestId } = render(
      <CustomerRow customer={NO_NAME} onPress={jest.fn()} currencyCode="AUD" />,
    );
    const style = StyleSheet.flatten(
      getByTestId("customer-row-c-real-monogram").props.style,
    );
    expect(style.borderRadius).toBe(theme.radii.md);
    expect(style.borderRadius).not.toBe(style.width / 2);
  });
});
