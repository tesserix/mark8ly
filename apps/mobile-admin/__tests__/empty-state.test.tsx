import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";

import { EmptyState } from "@/components/ui/EmptyState";

/**
 * Guards the project invariant "shared primitives take ADDITIVE props, never
 * changed defaults" at the one place it is actually load-bearing and was
 * entirely unenforced: `EmptyState.align`.
 *
 * `align` was added so the two editorial screens built in this increment
 * (the Dashboard's queue empty state, and Orders) could be left-aligned
 * without restyling the ~30 pre-existing call sites that were designed
 * against the centred treatment. That promise is only worth anything if the
 * DEFAULT cannot drift: flipping `align = "center"` to `align = "left"`
 * silently re-lays-out every one of those call sites, and — measured before
 * this file existed — left all 822 tests green.
 *
 * This is the same failure mode that already cost this programme a review
 * round, when changing `Eyebrow`'s own default gutter rippled into ~15 call
 * sites including 5 screens the report claimed were untouched.
 */
describe("EmptyState — the align default is part of the contract", () => {
  it("centres by default, so the pre-existing call sites are unchanged", () => {
    const { getByTestId, getByText } = render(<EmptyState title="Nothing here" message="Try later" />);

    const container = StyleSheet.flatten(getByTestId("empty-state").props.style);
    expect(container.alignItems).toBe("center");

    // The cross-axis alignment and the TEXT alignment have to agree — a
    // centred box full of left-aligned text is its own defect. Read off
    // `className`, not `style`: `Text` maps `align` to a nativewind class,
    // and RNTL renders without the nativewind runtime, so `style` is
    // undefined here (the same reason this suite cannot assert colours).
    expect(getByText("Nothing here").props.className).toContain("text-center");
    expect(getByText("Try later").props.className).toContain("text-center");
  });

  it("left-aligns only when a caller explicitly opts in", () => {
    const { getByTestId, getByText } = render(
      <EmptyState title="Nothing here" message="Try later" align="left" />,
    );

    const container = StyleSheet.flatten(getByTestId("empty-state").props.style);
    expect(container.alignItems).toBe("flex-start");
    expect(getByText("Nothing here").props.className).toContain("text-left");
    expect(getByText("Try later").props.className).toContain("text-left");
  });
});
