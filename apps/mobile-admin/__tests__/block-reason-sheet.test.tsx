// The reason collector for the ONE state-changing action Customers has.
//
// `BlockCustomerRequest.Reason` is `binding:"required"` server-side, so an
// empty reason is an unconditional 400 — and a 400 the merchant could have
// been shown inline is a failure of the client, not of the API. The submit
// gate below is therefore a contract test, not a nicety.
//
// The other two contracts pinned here are the ones that were WRONG in the
// version of this sheet that shipped with the customer detail screen:
//
//  1. it dismissed itself on submit, so a failed block closed the sheet and
//     threw away the typed reason with nothing said;
//  2. it had no `onDismiss`, so a parent had no way to learn that a merchant
//     backed out — which is exactly the stale-target bug Orders shipped.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { createRef, type ComponentProps } from "react";
import { act, fireEvent, render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { BottomSheetScrollView } from "@gorhom/bottom-sheet";
import {
  BlockReasonSheet,
  type BlockReasonSheetHandle,
} from "@/components/customers/BlockReasonSheet";
import { theme } from "@/lib/theme";

type SheetProps = ComponentProps<typeof BlockReasonSheet>;

function renderSheet(over: Partial<SheetProps> = {}) {
  const onSubmit = jest.fn();
  const onDismiss = jest.fn();
  const ref = createRef<BlockReasonSheetHandle>();
  const utils = render(
    <BlockReasonSheet
      ref={ref}
      customerLabel="Ada Lovelace"
      onSubmit={onSubmit}
      isSubmitting={false}
      error={null}
      onDismiss={onDismiss}
      {...over}
    />,
  );
  return { ...utils, onSubmit, onDismiss, ref };
}

describe("BlockReasonSheet", () => {
  it("keeps submit disabled while the reason is empty", () => {
    const { getByLabelText, onSubmit } = renderSheet();
    const submit = getByLabelText("Block customer");
    expect(submit.props.accessibilityState.disabled).toBe(true);
    fireEvent.press(submit);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // `strings.TrimSpace` is not what `binding:"required"` does — a reason of
  // "   " passes Go's required check and stores three spaces as the record of
  // WHY a customer was blocked. Treated as empty here instead.
  it("treats a whitespace-only reason as empty", () => {
    const { getByLabelText, onSubmit } = renderSheet();
    fireEvent.changeText(getByLabelText("Block reason"), "   ");
    const submit = getByLabelText("Block customer");
    expect(submit.props.accessibilityState.disabled).toBe(true);
    fireEvent.press(submit);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("passes the trimmed reason to onSubmit", () => {
    const { getByLabelText, onSubmit } = renderSheet();
    fireEvent.changeText(getByLabelText("Block reason"), "  Repeated chargebacks  ");
    fireEvent.press(getByLabelText("Block customer"));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit).toHaveBeenCalledWith("Repeated chargebacks");
  });

  // The parent dismisses once ITS mutation resolves. A sheet that closes on
  // submit has already discarded the typed reason by the time the request
  // fails, which is the whole reason `error` exists.
  it("does not dismiss itself on submit", () => {
    const { getByLabelText, onDismiss } = renderSheet();
    fireEvent.changeText(getByLabelText("Block reason"), "Fraud");
    fireEvent.press(getByLabelText("Block customer"));
    expect(onDismiss).not.toHaveBeenCalled();
    // Still open and still holding the reason, so a retry costs no retyping.
    expect(getByLabelText("Block reason").props.value).toBe("Fraud");
  });

  it("renders the error it is GIVEN", () => {
    const { getByText } = renderSheet({ error: "Couldn't block this customer. Try again." });
    expect(getByText("Couldn't block this customer. Try again.")).toBeTruthy();
  });

  // The sheet has no access to a mutation and must never grow one: react-query
  // never resets a mutation error, so a sheet bound to `mutation.error` greets
  // the merchant with the LAST customer's failure on the next customer.
  it("shows nothing when the error prop is null", () => {
    const { queryByText } = renderSheet({ error: null });
    expect(queryByText(/Try again/)).toBeNull();
  });

  it("clears the typed reason on every present()", () => {
    const { ref, getByLabelText } = renderSheet();
    fireEvent.changeText(getByLabelText("Block reason"), "leftover");
    act(() => ref.current?.present());
    expect(getByLabelText("Block reason").props.value).toBe("");
  });

  it("fires onDismiss when the merchant backs out", () => {
    const { getByLabelText, onDismiss } = renderSheet();
    fireEvent.press(getByLabelText("Cancel"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it("fires onDismiss when the parent dismisses it imperatively", () => {
    const { ref, onDismiss } = renderSheet();
    act(() => ref.current?.dismiss());
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  /**
   * The identity is a LINE OF ITS OWN, clamped to one line and allowed to
   * shrink — not a name interpolated into a sentence.
   *
   * A customer with no name is identified by their email, and an email is a
   * single indivisible token: wrapped inside running copy,
   * `mahesh.sangawar@gmail.com` breaks as `mahesh.sangawar@gmai` / `l.com`,
   * which reads as if the address ended at the line break. That exact break
   * is what the customer detail screen had to fix once already.
   */
  it("names the customer on one unbreakable line", () => {
    const { getByTestId } = renderSheet({ customerLabel: "mahesh.sangawar@gmail.com" });
    const line = getByTestId("block-sheet-customer");
    expect(line).toHaveTextContent("mahesh.sangawar@gmail.com");
    expect(line.props.numberOfLines).toBe(1);
    expect(line.props.adjustsFontSizeToFit).toBe(true);
  });

  it("omits the identity line entirely when no customer is armed", () => {
    const { queryByTestId } = renderSheet({ customerLabel: undefined });
    expect(queryByTestId("block-sheet-customer")).toBeNull();
  });

  /**
   * Caught on device at `content_size accessibility-large`: the body copy
   * alone grew past the sheet's fixed `52%` snap point and BOTH buttons were
   * cut off by the sheet's own bounds, with no gesture that could reveal
   * them. A percentage snap point cannot track Dynamic Type; a scrolling body
   * inside the bounded sheet can.
   *
   * Asserted on the SCROLL CONTAINER, not on a screenshot: the clipping is
   * invisible above the fold and reappears the moment someone "simplifies"
   * this back to a plain `View`.
   */
  it("puts the body in a scroll container so the actions stay reachable at any text size", () => {
    const { UNSAFE_root } = renderSheet();
    // By TYPE, not testID: the shared gorhom mock renders its scroll view as
    // a bare fragment, so there is no host node to query — but the element
    // and its props are still in the tree.
    const bodies = UNSAFE_root.findAllByType(BottomSheetScrollView);
    expect(bodies).toHaveLength(1);
    // The content container must not be flexed to the viewport — that is what
    // pinned the content to the sheet height in the first place.
    const style = StyleSheet.flatten(bodies[0].props.contentContainerStyle);
    expect(style?.flex).toBeUndefined();
    expect(style?.padding).toBe(theme.spacing.lg);
  });

  it("locks the field and the submit while the block is in flight", () => {
    const { getByLabelText, rerender, onSubmit } = renderSheet();
    fireEvent.changeText(getByLabelText("Block reason"), "Fraud");
    rerender(
      <BlockReasonSheet
        customerLabel="Ada Lovelace"
        onSubmit={onSubmit}
        isSubmitting
        error={null}
        onDismiss={() => {}}
      />,
    );
    expect(getByLabelText("Block reason").props.editable).toBe(false);
    const submit = getByLabelText("Block customer");
    expect(submit.props.accessibilityState.disabled).toBe(true);
    fireEvent.press(submit);
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
