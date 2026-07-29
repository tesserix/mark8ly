// Tickets = the support queue, brought onto the increment-3 native-UX kit
// (inc3 Task 5): a collapsing header with a back affordance, a single gated
// Close swipe, and a two-item long-press menu.
//
// 🔴 THE ONE SCREEN IN THIS INCREMENT WHERE THE ACTION IS NOT IDEMPOTENT AND
// THE GATE IS LOAD-BEARING. `PATCH .../tickets/:id {status:"closed"}` on an
// already-closed ticket is refused: `CanTransitionTo` returns false for a
// same-status target AND `closed` is terminal (ticket/models.go:112-133), and
// `CodeInvalidTransition` maps to HTTP 409 (handlers/admin/errors.go:38).
// There is no reopen endpoint.
//
// So, unlike Products and Reviews where a missing gate merely produces a
// pointless no-op, a missing gate here produces a visible server error on a
// row the merchant cannot repair. That is why:
//
//  1. The CLOSED fixture is mandatory. Without it the 409-avoidance gate is
//     untested and a store with only open tickets passes against completely
//     broken code.
//  2. Close is CONFIRMED even though it is a revealed-then-tapped swipe.
//     Everywhere else in this plan a revealed tap is enough because the
//     action is reversible. This one is not; one extra tap is the price.
//  3. Close is `neutral` on the swipe and `danger` in the sheet, and BOTH
//     tones are asserted. Closing a resolved ticket is a normal outcome, not
//     a destruction — the trailing edge is a POSITION, not a tone. In the
//     sheet, where no side carries meaning, `danger` marks the one-way action.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`) — the
// global one carries neither `Animated.FlatList` (which drives the
// CollapsingHeader) nor the `FadeIn` entering animation in full. Same shape
// as products-screen.test.tsx's. A test-environment gap, not a semantics
// change.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
  class ChainableAnimation {
    duration() {
      return this;
    }
    easing() {
      return this;
    }
  }
  function interpolate(
    value: number,
    inputRange: [number, number],
    outputRange: [number, number],
  ) {
    const [inMin, inMax] = inputRange;
    const [outMin, outMax] = outputRange;
    const t = Math.max(0, Math.min(1, (value - inMin) / (inMax - inMin)));
    return outMin + t * (outMax - outMin);
  }
  return {
    __esModule: true,
    default: { View, FlatList },
    FadeIn: new ChainableAnimation(),
    Easing: { bezier: () => (t: number) => t },
    Extrapolation: { CLAMP: "clamp", EXTEND: "extend", IDENTITY: "identity" },
    interpolate,
    useAnimatedStyle: (factory: () => unknown) => factory(),
    useDerivedValue: (factory: () => number) => ({ value: factory() }),
    useAnimatedScrollHandler: (handler: unknown) => handler,
    useSharedValue: (initial: number) => {
      const ref = useRef(undefined) as { current: { value: number } | undefined };
      if (ref.current === undefined) ref.current = { value: initial };
      return ref.current;
    },
    runOnJS: (fn: (...args: unknown[]) => unknown) => fn,
    withSpring: (toValue: number) => toValue,
    withTiming: (toValue: number) => toValue,
    useReducedMotion: jest.fn(() => false),
  };
});

const mockPush = jest.fn();
const mockBack = jest.fn();
jest.mock("expo-router", () => ({
  useRouter: () => ({ push: mockPush, back: mockBack }),
}));

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

// --- controllable ticket list -------------------------------------------
interface ListParams {
  status?: string;
}
const mockListCalls: (ListParams | undefined)[] = [];
let mockTickets: unknown[] = [];
let mockIsError = false;
let mockIsLoading = false;
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-tickets", () => ({
  useTickets: (params?: ListParams) => {
    mockListCalls.push(params);
    const rows = params?.status
      ? mockTickets.filter((t) => (t as { status: string }).status === params.status)
      : mockTickets;
    return {
      data: mockIsError
        ? undefined
        : { pages: [{ data: rows, meta: { page: 1, page_size: 20, total: rows.length, total_pages: 1 } }] },
      isLoading: mockIsLoading,
      isRefetching: false,
      isError: mockIsError,
      refetch: mockRefetch,
      fetchNextPage: jest.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
  },
}));

const mockUpdateStatus = jest.fn();
jest.mock("@/lib/admin-api/ticket-actions", () => ({
  useUpdateTicketStatus: () => ({ mutate: mockUpdateStatus, isPending: false }),
}));

import { act, fireEvent, render, within } from "@testing-library/react-native";
import { Alert, StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import TicketsScreen, { TicketRow } from "../app/(tabs)/more/settings/tickets/index";
import { theme } from "@/lib/theme";
import {
  DESTRUCTIVE_TONE,
  assertNoAutoFire,
  assertSwipeConvention,
  swipeRow,
  type Root,
  type RowActions,
} from "../test-utils/swipe-convention";
import type { Ticket } from "@repo/mobile-shared/api/types";

function ticket(over: Partial<Ticket> = {}): Ticket {
  return {
    id: "t-open",
    ticket_number: "SUP-1001",
    subject: "Parcel never arrived",
    description: "Tracking has not moved in eight days.",
    status: "open",
    priority: "normal",
    submitted_by_name: "Sofia Reyes",
    submitted_by_email: "sofia@example.com",
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    replies: [],
    ...over,
  } as unknown as Ticket;
}

const OPEN = ticket();
// 🔴 MANDATORY. `closed` is terminal and re-closing 409s, so this is the
// fixture that makes the gate falsifiable at all — every assertion about
// "cannot be closed twice" is vacuous without a closed row on screen.
const CLOSED = ticket({
  id: "t-closed",
  ticket_number: "SUP-0990",
  subject: "Refund processed",
  status: "closed",
});

beforeEach(() => {
  jest.clearAllMocks();
  mockListCalls.length = 0;
  mockTickets = [OPEN, CLOSED];
  mockIsError = false;
  mockIsLoading = false;
});

function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`ticket-row-${id}`) as never, "longPress");
}

function ticketRows(root: Root) {
  return root.findAllByType(TicketRow);
}

/**
 * One row's mounted `TicketRow` element.
 *
 * The busy guard is asserted on `onLongPress` THROUGH THIS, not through
 * "firing longPress produced no sheet": `fireEvent(el, "longPress")` against
 * an element with no handler is a silent no-op, so that proxy passes both
 * when the guard holds and when the event never dispatched at all.
 */
function ticketRow(root: Root, id: string) {
  return ticketRows(root).find((n) => (n.props as { ticket: Ticket }).ticket.id === id);
}

/** The destructive button the Close confirm hands to `Alert.alert`. */
function closeConfirmButton(spy: jest.SpyInstance) {
  const buttons = spy.mock.calls[0]?.[2] as
    | { text: string; style?: string; onPress?: () => void }[]
    | undefined;
  return buttons?.find((b) => b.style === "destructive");
}

describe("Tickets — header", () => {
  it("titles the screen with the editorial collapsing header", () => {
    const { getAllByText } = render(<TicketsScreen />);
    expect(getAllByText("Tickets").length).toBeGreaterThan(0);
    expect(getAllByText("SUPPORT").length).toBeGreaterThan(0);
  });

  it("keeps a back affordance, in its own nav row", () => {
    const { getByTestId } = render(<TicketsScreen />);
    expect(getByTestId("collapsing-header-nav-row")).toBeTruthy();
    expect(getByTestId("collapsing-header-leading")).toBeTruthy();
  });

  it("goes back when the chevron is tapped", () => {
    const { getByLabelText } = render(<TicketsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  // The Plus moved from `BackHeader.rightSlot` to `CollapsingHeader.rightSlot`
  // and must survive the move — raising a ticket is the only write path on
  // this screen that is not a status change.
  it("keeps the new-ticket button reachable in the right slot", () => {
    const { getByLabelText } = render(<TicketsScreen />);
    fireEvent.press(getByLabelText("New ticket"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/settings/tickets/new");
  });

  it("drives the header from the list's own scroll offset", () => {
    const { getByTestId } = render(<TicketsScreen />);
    const list = getByTestId("tickets-list");
    expect(list.props.onScroll).toBeTruthy();
    expect(list.props.scrollEventThrottle).toBe(16);
    expect(list.props.ListHeaderComponent).toBeFalsy();
  });
});

describe("Tickets — swipe", () => {
  it("puts Close on the trailing edge in the NEUTRAL tone, with nothing leading", () => {
    const row = swipeRow(render(<TicketsScreen />).UNSAFE_root, "swipe-t-open")
      ?.props as RowActions;
    // §3 gives tickets no constructive swipe: there is no action on this
    // screen that ADDS anything, so the leading edge stays unarmed.
    expect(row.leadingActions ?? []).toHaveLength(0);
    expect(row.trailingActions).toHaveLength(1);
    expect(row.trailingActions?.[0]).toMatchObject({ key: "close", tone: "neutral" });
  });

  // The row that proves the app-wide invariant test isn't merely "trailing
  // means danger". Closing a resolved ticket is a normal outcome.
  it("does not paint Close as destructive on the swipe", () => {
    const row = swipeRow(render(<TicketsScreen />).UNSAFE_root, "swipe-t-open")
      ?.props as RowActions;
    expect(row.trailingActions?.[0].tone).not.toBe(DESTRUCTIVE_TONE);
  });

  it("paints Close in the neutral sink, not oxblood", () => {
    const { getByTestId } = render(<TicketsScreen />);
    const close = StyleSheet.flatten(getByTestId("swipe-t-open-action-close").props.style);
    expect(close.backgroundColor).toBe(theme.colors.sink);
    expect(close.backgroundColor).not.toBe(theme.colors.danger);
  });

  // 🔴 The load-bearing gate. Closing a closed ticket is an HTTP 409, and
  // there is no way back — so the gesture is not merely inert, it is ABSENT.
  it("mounts NO SwipeRow on a closed ticket", () => {
    const { UNSAFE_root } = render(<TicketsScreen />);
    expect(swipeRow(UNSAFE_root, "swipe-t-closed")).toBeUndefined();
  });

  it("holds the app-wide swipe convention", () => {
    assertSwipeConvention(render(<TicketsScreen />).UNSAFE_root);
  });

  it("never opts any action into full-swipe auto-fire", () => {
    assertNoAutoFire(render(<TicketsScreen />).UNSAFE_root);
  });

  it("does not fire the mutation until the confirm is accepted", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<TicketsScreen />);

    fireEvent.press(getByTestId("swipe-t-open-action-close"));
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockUpdateStatus).not.toHaveBeenCalled();

    act(() => closeConfirmButton(alertSpy)?.onPress?.());
    expect(mockUpdateStatus).toHaveBeenCalledTimes(1);
    expect(mockUpdateStatus.mock.calls[0][0]).toEqual({ id: "t-open", status: "closed" });
    alertSpy.mockRestore();
  });

  // Cancelling must be inert — it carries no onPress at all, so there is
  // nothing that could fire even by accident.
  it("does nothing at all when the confirm is cancelled", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<TicketsScreen />);
    fireEvent.press(getByTestId("swipe-t-open-action-close"));

    const buttons = alertSpy.mock.calls[0][2] as {
      text: string;
      style?: string;
      onPress?: () => void;
    }[];
    const cancel = buttons.find((b) => b.style === "cancel");
    expect(cancel).toBeTruthy();
    expect(cancel?.onPress).toBeUndefined();
    expect(mockUpdateStatus).not.toHaveBeenCalled();
    alertSpy.mockRestore();
  });

  // The copy is the only place the merchant is told this is one-way. If it
  // ever softens, the confirm stops earning the extra tap it costs.
  it("tells the merchant closing cannot be undone", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<TicketsScreen />);
    fireEvent.press(getByTestId("swipe-t-open-action-close"));
    expect(alertSpy.mock.calls[0][1]).toMatch(/reopen|undone|permanent/i);
    expect(alertSpy.mock.calls[0][1]).toContain("SUP-1001");
    alertSpy.mockRestore();
  });
});

describe("Tickets — long-press menu", () => {
  const KEYS = ["reply", "close"];

  // `snapPoints` memoises on `items.length`, so a dropped item resizes the
  // sheet under the merchant's thumb. Assign is CUT — there is no assignee
  // model and no route. Two items.
  it("always renders two items so the sheet never resizes", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<TicketsScreen />);

    longPress(getByTestId, "t-open");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(2);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();

    fireEvent.press(getByTestId("action-sheet-item-reply"));
    expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    // Same two on the CLOSED ticket, where one of them is illegal.
    longPress(getByTestId, "t-closed");
    expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(2);
    for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
  });

  // 🔴 The other half of the 409 gate. A closed ticket keeps the item —
  // dropping it would resize the sheet — but it cannot be fired.
  it("disables Close in the sheet on a closed ticket rather than dropping it", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<TicketsScreen />);
    longPress(getByTestId, "t-closed");
    expect(getByTestId("action-sheet-item-close").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(getByTestId("action-sheet-item-close"));
    // Neither the confirm NOR the mutation — a disabled item must not even
    // reach the Alert.
    expect(alertSpy).not.toHaveBeenCalled();
    expect(mockUpdateStatus).not.toHaveBeenCalled();
    alertSpy.mockRestore();
  });

  // In the sheet there is no side to carry meaning, so the tone does the
  // work the trailing edge does on the row: this is the one-way action.
  it("paints Close as the destructive item in the sheet", () => {
    const { getByTestId } = render(<TicketsScreen />);
    longPress(getByTestId, "t-open");
    const closeItem = within(getByTestId("action-sheet-item-close")).getByText("Close");
    expect(StyleSheet.flatten(closeItem.props.style).color).toBe(theme.colors.danger);
    const replyItem = within(getByTestId("action-sheet-item-reply")).getByText("Reply");
    expect(StyleSheet.flatten(replyItem.props.style)?.color).not.toBe(theme.colors.danger);
  });

  it("confirms Close from the menu too", () => {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const { getByTestId } = render(<TicketsScreen />);
    longPress(getByTestId, "t-open");
    fireEvent.press(getByTestId("action-sheet-item-close"));
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockUpdateStatus).not.toHaveBeenCalled();

    act(() => closeConfirmButton(alertSpy)?.onPress?.());
    expect(mockUpdateStatus.mock.calls[0][0]).toEqual({ id: "t-open", status: "closed" });
    alertSpy.mockRestore();
  });

  it("Reply opens the ticket detail, where the reply box lives", () => {
    const { getByTestId } = render(<TicketsScreen />);
    longPress(getByTestId, "t-open");
    fireEvent.press(getByTestId("action-sheet-item-reply"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/settings/tickets/t-open");
  });
});

describe("Tickets — no optimistic hide", () => {
  function closeOpenTicket(getByTestId: (id: string) => never) {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    fireEvent.press(getByTestId("swipe-t-open-action-close"));
    act(() => closeConfirmButton(alertSpy)?.onPress?.());
    alertSpy.mockRestore();
  }

  it("leaves the row on screen after Close", () => {
    const { getByTestId } = render(<TicketsScreen />);
    closeOpenTicket(getByTestId as never);
    expect(getByTestId("ticket-row-t-open")).toBeTruthy();
  });

  it("suppresses BOTH the swipe and the long-press while that row's request is open", () => {
    const { getByTestId, queryByTestId, UNSAFE_root } = render(<TicketsScreen />);
    closeOpenTicket(getByTestId as never);

    expect(swipeRow(UNSAFE_root, "swipe-t-open")?.props.enabled).toBe(false);
    // Asserted DIRECTLY on the prop — `SwipeRow.enabled` does not reach
    // `onLongPress`, which is the whole reason both are named separately.
    expect(ticketRow(UNSAFE_root, "t-open")?.props.onLongPress).toBeUndefined();
    longPress(getByTestId, "t-open");
    expect(queryByTestId("action-sheet-item-close")).toBeNull();

    // A DIFFERENT row is untouched — the guard is per row.
    expect(ticketRow(UNSAFE_root, "t-closed")?.props.onLongPress).toBeDefined();
    longPress(getByTestId, "t-closed");
    expect(getByTestId("action-sheet-item-close")).toBeTruthy();
  });

  it("re-enables the row once the mutation settles", () => {
    const { getByTestId, UNSAFE_root } = render(<TicketsScreen />);
    closeOpenTicket(getByTestId as never);
    act(() => (mockUpdateStatus.mock.calls[0][1].onSuccess as () => void)());

    expect(swipeRow(UNSAFE_root, "swipe-t-open")?.props.enabled).not.toBe(false);
    expect(ticketRow(UNSAFE_root, "t-open")?.props.onLongPress).toBeDefined();
  });

  it("releases a row on failure too, so a failed close is retryable", () => {
    const { getByTestId, UNSAFE_root } = render(<TicketsScreen />);
    closeOpenTicket(getByTestId as never);
    expect(swipeRow(UNSAFE_root, "swipe-t-open")?.props.enabled).toBe(false);

    act(() =>
      (mockUpdateStatus.mock.calls[0][1].onError as (e: Error) => void)(new Error("409")),
    );
    expect(swipeRow(UNSAFE_root, "swipe-t-open")?.props.enabled).not.toBe(false);
  });

  it("fires the success and failure haptics exactly once each", () => {
    const { getByTestId } = render(<TicketsScreen />);
    closeOpenTicket(getByTestId as never);
    act(() => (mockUpdateStatus.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    closeOpenTicket(getByTestId as never);
    act(() => (mockUpdateStatus.mock.calls[1][1].onError as (e: Error) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });
});

/**
 * Task 14 retrofit — a failed close is EXPLAINED, not only felt.
 *
 * This screen has more need of it than any other in the increment: a 409 is
 * the one server refusal the merchant can actually hit here, and without a
 * message a refused close is a haptic and a badge that still reads Open.
 */
describe("Tickets — surfacing a failed close", () => {
  function apiError(status: number, code: string, message: string): Error {
    const err = new Error(message) as Error & { status: number; code: string };
    err.name = "ApiError";
    err.status = status;
    err.code = code;
    return err;
  }

  function failClose(root: ReturnType<typeof render>, error: unknown) {
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    fireEvent.press(root.getByTestId("swipe-t-open-action-close"));
    act(() => closeConfirmButton(alertSpy)?.onPress?.());
    alertSpy.mockRestore();
    act(() =>
      (mockUpdateStatus.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(error),
    );
  }

  it("says nothing at all until something fails", () => {
    const root = render(<TicketsScreen />);
    const alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    fireEvent.press(root.getByTestId("swipe-t-open-action-close"));
    act(() => closeConfirmButton(alertSpy)?.onPress?.());
    alertSpy.mockRestore();
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  it("names the action the merchant actually took", () => {
    const root = render(<TicketsScreen />);
    failClose(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't close this ticket",
    );
  });

  // The 409 this screen's whole gate exists to avoid. If the gate is ever
  // bypassed the merchant at least reads the server's own account of it.
  it("prefers the server's own words on a refused transition", () => {
    const root = render(<TicketsScreen />);
    failClose(root, apiError(409, "invalid_transition", "cannot transition from closed to closed"));
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(
      /cannot transition from closed to closed/,
    );
  });
});

describe("Tickets — filters and rows", () => {
  it("renders the full filter set as pill chips", () => {
    const { getByTestId } = render(<TicketsScreen />);
    for (const key of ["all", "open", "resolved", "closed"]) {
      expect(getByTestId(`filter-chip-${key}`)).toBeTruthy();
    }
  });

  it("asks the backend for status=closed when that chip is tapped", () => {
    const { getByTestId } = render(<TicketsScreen />);
    mockListCalls.length = 0;
    fireEvent.press(getByTestId("filter-chip-closed-target"));
    expect(mockListCalls).toContainEqual({ status: "closed" });
  });

  it("opens the ticket on tap", () => {
    const { getByTestId } = render(<TicketsScreen />);
    fireEvent.press(getByTestId("ticket-row-t-open"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/settings/tickets/t-open");
  });
});
