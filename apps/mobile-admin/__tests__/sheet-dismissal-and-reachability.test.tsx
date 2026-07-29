// The two guarantees every imperative reason/composer sheet in this app owes
// the merchant, pinned across all four of them at once.
//
//  1. REACHABILITY. Each sheet pairs a fixed PERCENTAGE snap point with its
//     content. A percentage cannot track Dynamic Type, so at
//     `content_size accessibility-large` the body copy alone outgrows the
//     sheet and BOTH action buttons are cut off by the sheet's own bounds —
//     with no gesture that can reveal them. That was found on device against
//     `BlockReasonSheet` and fixed there; `CancelReasonSheet`,
//     `RefundSheet` and `EmailLabelSheet` shipped with the identical trap,
//     and two of the three gate money (cancel, refund). A merchant at
//     accessibility text sizes could not cancel or refund an order at all.
//
//  2. DISMISSAL. `BottomSheetModal` has no default backdrop and gorhom's
//     hosting container is `pointerEvents: "box-none"`, so without one the
//     area above the sheet stays live: over the Orders/Customers LIST a
//     mis-tap lands on a row and navigates away mid-sentence, discarding
//     what was typed. And once a backdrop exists, every dismissal route
//     (button, swipe, backdrop) must agree with the others about whether
//     backing out is allowed while a submit is in flight — a disabled
//     "Cancel" button next to a live swipe-down is a lie.
//
// Asserted on the component tree, NOT on a screenshot: RNTL performs no
// layout, so the clipping in (1) is literally unobservable here. What IS
// observable — and what regresses the moment someone "simplifies" a sheet
// back to a plain `View` — is the shape that makes the content reachable at
// every text size. The AX-size render itself is verified on device.

// A LOCAL mock, not the shared lib/test-support/gorhom-bottom-sheet-mock:
// that one renders `children` unconditionally, never invokes
// `backdropComponent`, and ignores `enablePanDownToClose` — so every
// assertion below would hold regardless of what the sheets actually pass.
// A test file's own `jest.mock()` wins over jest.config.js's
// `moduleNameMapper`, so this factory applies to this suite only.
//
// It models the three parts of @gorhom/bottom-sheet 5.x this suite is about,
// read off the library's own source:
//
//   * `BottomSheetBackdrop` attaches its tap gesture ONLY when
//     `pressBehavior !== "none"` (see BottomSheetBackdrop.tsx's
//     `return pressBehavior !== 'none' ? <GestureDetector …> : AnimatedView`).
//     The scrim itself keeps `pointerEvents: "auto"` either way, which is why
//     "none" is the right way to disarm a backdrop without dropping the
//     tap-through shield.
//   * `enablePanDownToClose` decides whether a downward pan can close the
//     sheet at all.
//   * `onDismiss` fires once per close, whatever triggered it.
//
// There is deliberately no hardware-back path modelled: @gorhom/bottom-sheet
// 5.2.14 registers no `BackHandler` anywhere, and `BottomSheetModal` renders
// through a portal rather than a react-native `Modal`, so it has no
// `onRequestClose` either. The button, the swipe and the backdrop are the
// complete set of dismissal routes.
//
// Written without JSX and without a literal `createElement` identifier —
// nativewind's babel transform rewrites both into a module-scope helper
// import, and jest.mock() factories are hoisted above it ("module factory is
// not allowed to reference any out-of-scope variables"). Destructuring
// `createElement` under another local name defeats that literal-name check
// while still producing real elements. Same dodge as action-sheet.test.tsx.
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  const { createElement: h } = require("react");
  const { Pressable, View } = require("react-native");

  const BottomSheetBackdrop = (props: Record<string, unknown>) => {
    const { pressBehavior, close, style } = props as {
      pressBehavior?: string;
      close?: () => void;
      style?: unknown;
    };
    return h(Pressable, {
      testID: "sheet-backdrop",
      accessibilityRole: "button",
      accessibilityLabel: "Close sheet",
      style,
      // Mirrors the real component: no tap gesture at all when
      // `pressBehavior === "none"`, so a press is inert rather than closing.
      onPress: () => {
        if (pressBehavior !== "none") close?.();
      },
    });
  };

  const BottomSheetScrollView = (props: {
    children?: React.ReactNode;
    contentContainerStyle?: unknown;
    testID?: string;
  }) => h(React.Fragment, null, props.children ?? null);

  const BottomSheetView = (props: { children?: React.ReactNode }) =>
    h(React.Fragment, null, props.children ?? null);

  const BottomSheetModal = React.forwardRef(
    (
      props: {
        children?: React.ReactNode;
        onDismiss?: () => void;
        backdropComponent?: (p: Record<string, unknown>) => React.ReactNode;
        enablePanDownToClose?: boolean;
      },
      ref: React.Ref<unknown>,
    ) => {
      const { children, onDismiss, backdropComponent, enablePanDownToClose } = props;
      const [presented, setPresented] = React.useState(false);
      const presentedRef = React.useRef(false);

      const close = React.useCallback(() => {
        if (!presentedRef.current) return;
        presentedRef.current = false;
        setPresented(false);
        onDismiss?.();
      }, [onDismiss]);

      React.useImperativeHandle(ref, () => ({
        present: () => {
          presentedRef.current = true;
          setPresented(true);
        },
        // The parent's imperative dismiss after a successful mutation. It is
        // NOT gated on anything — gating it would break the success path.
        dismiss: close,
      }));

      if (!presented) return null;

      const backdrop = backdropComponent ? backdropComponent({ close }) : null;
      // Stands in for the downward pan. Real gorhom only closes on that pan
      // when `enablePanDownToClose` is true; below that flag the sheet
      // springs back to its snap point instead.
      const panDown = h(Pressable, {
        testID: "sheet-pan-down",
        onPress: () => {
          if (enablePanDownToClose) close();
        },
      });
      return h(View, { testID: "sheet-host" }, backdrop, panDown, children ?? null);
    },
  );

  return {
    __esModule: true,
    BottomSheetModal,
    BottomSheetModalProvider: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetBackdrop,
    BottomSheetScrollView,
    BottomSheetView,
  };
});

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import {
  createRef,
  type FunctionComponent,
  type ReactElement,
  type ReactNode,
  type Ref,
} from "react";
import { act, fireEvent, render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import { BottomSheetScrollView } from "@gorhom/bottom-sheet";
import {
  BlockReasonSheet,
  type BlockReasonSheetHandle,
} from "@/components/customers/BlockReasonSheet";
import {
  CancelReasonSheet,
  type CancelReasonSheetHandle,
} from "@/components/orders/CancelReasonSheet";
import { RefundSheet, type RefundSheetHandle } from "@/components/orders/RefundSheet";
import {
  EmailLabelSheet,
  type EmailLabelSheetHandle,
} from "@/components/orders/EmailLabelSheet";
import { theme } from "@/lib/theme";

// Same TS2786 duplicate-`@types/react` dodge the sheets themselves use for
// `BottomSheetScrollView`, whose real type is incompatible with
// `findAllByType`'s signature under this project's React types. Narrowed to
// `FunctionComponent` rather than `ComponentType`: the class half of
// `ComponentType` drags in gorhom's own `React.Context`, which is a different
// physical type from this project's and fails to satisfy `ElementType`.
const ScrollBody = BottomSheetScrollView as unknown as FunctionComponent<{
  children?: ReactNode;
  contentContainerStyle?: unknown;
}>;

/** The imperative handle every sheet here exposes for opening itself. */
interface PresentableHandle {
  present: () => void;
}

interface SheetCase {
  name: string;
  /** Fills the sheet in so its submit is armed, then presses it. */
  submit: (utils: ReturnType<typeof render>) => void;
  render: (args: {
    isSubmitting: boolean;
    onSubmit: jest.Mock;
    onDismiss: jest.Mock;
    ref: Ref<PresentableHandle>;
  }) => ReactElement;
}

const cases: SheetCase[] = [
  {
    name: "BlockReasonSheet",
    submit: ({ getByLabelText }) => {
      fireEvent.changeText(getByLabelText("Block reason"), "Repeated chargebacks");
      fireEvent.press(getByLabelText("Block customer"));
    },
    render: ({ isSubmitting, onSubmit, onDismiss, ref }) => (
      <BlockReasonSheet
        ref={ref as Ref<BlockReasonSheetHandle>}
        customerLabel="Ada Lovelace"
        onSubmit={onSubmit}
        isSubmitting={isSubmitting}
        error={null}
        onDismiss={onDismiss}
      />
    ),
  },
  {
    name: "CancelReasonSheet",
    submit: ({ getByLabelText }) => {
      fireEvent.changeText(getByLabelText("Cancellation reason"), "Duplicate order");
      fireEvent.press(getByLabelText("Cancel order"));
    },
    render: ({ isSubmitting, onSubmit, onDismiss, ref }) => (
      <CancelReasonSheet
        ref={ref as Ref<CancelReasonSheetHandle>}
        onSubmit={onSubmit}
        isSubmitting={isSubmitting}
        hasShipment
        carrier="delhivery"
        error={null}
        onDismiss={onDismiss}
      />
    ),
  },
  {
    name: "RefundSheet",
    submit: ({ getByLabelText }) => {
      fireEvent.press(getByLabelText("Issue refund"));
    },
    render: ({ isSubmitting, onSubmit, onDismiss, ref }) => (
      <RefundSheet
        ref={ref as Ref<RefundSheetHandle>}
        onSubmit={onSubmit}
        isSubmitting={isSubmitting}
        hasShipment
        refundableAmount={4250}
        currencyCode="AUD"
        error={null}
        onDismiss={onDismiss}
      />
    ),
  },
  {
    name: "EmailLabelSheet",
    submit: ({ getByLabelText }) => {
      fireEvent.changeText(getByLabelText("Recipient email"), "warehouse@example.com");
      fireEvent.press(getByLabelText("Send label"));
    },
    render: ({ isSubmitting, onSubmit, onDismiss, ref }) => (
      <EmailLabelSheet
        ref={ref as Ref<EmailLabelSheetHandle>}
        onSubmit={onSubmit}
        isSubmitting={isSubmitting}
        onDismiss={onDismiss}
      />
    ),
  },
];

describe.each(cases)("$name", ({ submit, render: renderSheet }) => {
  function open(isSubmitting = false) {
    const onSubmit = jest.fn();
    const onDismiss = jest.fn();
    const ref = createRef<PresentableHandle>();
    const utils = render(renderSheet({ isSubmitting, onSubmit, onDismiss, ref }));
    act(() => ref.current?.present());
    return { ...utils, onSubmit, onDismiss, ref };
  }

  /**
   * The reachability guarantee. Caught on device at
   * `content_size accessibility-large`: the body copy alone grew past the
   * sheet's fixed percentage snap point and BOTH action buttons were cut off
   * by the sheet's own bounds, with no gesture that could reveal them.
   *
   * Asserted on the SCROLL CONTAINER rather than on pixels because the
   * clipping is invisible above the fold — which is exactly why this shipped.
   */
  it("puts the body in a scroll container so both actions stay reachable at any text size", () => {
    const { UNSAFE_root } = open();
    // By TYPE, not testID: the mocked scroll view renders as a bare fragment,
    // so there is no host node to query — but the element and its props are
    // still in the tree.
    const bodies = UNSAFE_root.findAllByType(ScrollBody);
    expect(bodies).toHaveLength(1);

    const style = StyleSheet.flatten(
      (bodies[0].props as { contentContainerStyle?: unknown }).contentContainerStyle,
    ) as { flex?: number; padding?: number; paddingBottom?: number } | undefined;
    // A flexed CONTENT container pins the content to the viewport height,
    // which is the very thing that clipped the buttons — the fix is worthless
    // with `flex: 1` left on.
    expect(style?.flex).toBeUndefined();
    expect(style?.padding).toBe(theme.spacing.lg);
    // Runway below the last button so it is not flush against the sheet edge
    // once the body scrolls.
    expect(style?.paddingBottom).toBe(theme.spacing.xxl);
  });

  /**
   * Without a backdrop the area above the sheet stays live (gorhom's hosting
   * container is `pointerEvents: "box-none"`), so a mis-tap over the list
   * behind it navigates away and discards what was typed.
   */
  it("shields the screen behind it with a flat ink scrim", () => {
    const { getByTestId } = open();
    const scrim = getByTestId("sheet-backdrop");
    const style = StyleSheet.flatten(scrim.props.style) as
      | { backgroundColor?: string }
      | undefined;
    // A solid ink token, never a blur — this design system bans
    // glassmorphism, so a translucent material here would be a regression
    // even though it would still block the tap-through.
    expect(style?.backgroundColor).toBe(theme.colors.overlay);
  });

  it("closes on a backdrop tap without submitting anything", () => {
    const { getByTestId, onDismiss, onSubmit } = open();
    fireEvent.press(getByTestId("sheet-backdrop"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  /**
   * The three dismissal routes must agree. The button is disabled while a
   * submit is in flight; a backdrop that still closed would both contradict
   * that and land the mutation's settle handlers on a sheet whose target the
   * parent has already released.
   */
  it("leaves the backdrop inert while a submit is in flight", () => {
    const { getByTestId, onDismiss } = open(true);
    fireEvent.press(getByTestId("sheet-backdrop"));
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("cannot be swiped away while a submit is in flight", () => {
    const { getByTestId, onDismiss } = open(true);
    fireEvent.press(getByTestId("sheet-pan-down"));
    expect(onDismiss).not.toHaveBeenCalled();
  });

  // The gate above must be a gate, not a permanent lock: at rest the swipe is
  // still the fastest way out of the sheet.
  it("can still be swiped away at rest", () => {
    const { getByTestId, onDismiss } = open();
    fireEvent.press(getByTestId("sheet-pan-down"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  // Layout and dismissal only — the submit contract each sheet already owns
  // is unchanged, and this asserts nothing was disturbed on the way past it.
  it("still reaches its own submit", () => {
    const utils = open();
    submit(utils);
    expect(utils.onSubmit).toHaveBeenCalledTimes(1);
  });
});
