// Segments, brought onto the increment-3 native-UX kit (inc3 Task 7):
// collapsing header with a back chevron, and a two-item long-press menu whose
// only real action is an irreversible Delete.
//
// NO SWIPE and NO FILTER CHIPS. `DELETE /segments/:id` is a HARD delete with
// no undo, which disqualifies it from a one-thumb gesture; and the resource
// has no status axis at all, so there is nothing for a chip strip to filter.
// Both absences are ASSERTED below rather than left to be re-litigated.
//
// The delete is refused — `409 segment_in_use` — whenever a campaign still
// points at the segment (campaign/service.go:273-286, and again at the
// Postgres FK in campaign/repository.go:125-141 for the TOCTOU window). The
// server's message names the blocking campaign COUNT, a fact the client
// cannot reconstruct, so the fixtures include a segment a campaign targets
// and the assertions pin that prose through VERBATIM.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

// Local mock (it wins over `__mocks__/react-native-reanimated.js`): this
// screen drives its `CollapsingHeader` from an `Animated.FlatList`, which the
// global mock does not carry.
jest.mock("react-native-reanimated", () => {
  const { View, FlatList } = require("react-native");
  const { useRef } = require("react");
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

interface SegmentsQueryState {
  segments: Segment[];
  isLoading: boolean;
  isError: boolean;
}
const mockState: SegmentsQueryState = { segments: [], isLoading: false, isError: false };
const mockRefetch = jest.fn();

jest.mock("@/lib/hooks/use-segments", () => ({
  useSegments: () => ({
    // A bare `{data}` — segments.go:39 sends no meta and the list is not
    // paginated. The screen must not grow an infinite-query shape it has no
    // backend for.
    data: { data: mockState.segments },
    isLoading: mockState.isLoading,
    isRefetching: false,
    isError: mockState.isError,
    refetch: mockRefetch,
  }),
}));

const mockDelete = jest.fn();
jest.mock("@/lib/admin-api/segment-actions", () => ({
  useDeleteSegment: () => ({ mutate: mockDelete, isPending: false }),
}));

import { act, fireEvent, render } from "@testing-library/react-native";
import { Alert, StyleSheet } from "react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import SegmentsScreen from "../app/(tabs)/more/marketing/segments/index";
import { SegmentRow } from "@/components/marketing/SegmentRow";
import { theme } from "@/lib/theme";
import { swipeRows, type Root } from "../test-utils/swipe-convention";
import type { Segment } from "@repo/mobile-shared/api/types";

function segment(over: Partial<Segment> = {}): Segment {
  return {
    id: "seg-summer",
    name: "Summer buyers",
    description: "Bought between Nov and Feb",
    rules: "[]",
    member_count: 42,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  } as unknown as Segment;
}

/**
 * The segment a campaign points at. Nothing on the wire says so — the list
 * carries NO campaign-linkage field — which is exactly why Delete cannot be
 * gated client-side and why the server's refusal is the only thing that can
 * tell the merchant.
 */
const REFERENCED = segment();
/** Unreferenced: the delete the server will actually carry out. */
const FREE = segment({
  id: "seg-lapsed",
  name: "Lapsed customers",
  description: null,
  member_count: 7,
});

beforeEach(() => {
  jest.clearAllMocks();
  mockState.segments = [REFERENCED, FREE];
  mockState.isLoading = false;
  mockState.isError = false;
});

function longPress(getByTestId: (id: string) => unknown, id: string) {
  fireEvent(getByTestId(`segment-row-${id}`) as never, "longPress");
}

/**
 * One row's mounted `SegmentRow` element. The busy guard is read off
 * `onLongPress` THROUGH THIS — `fireEvent(el, "longPress")` against an
 * element with no handler is a silent no-op, so "no sheet appeared" passes
 * both when the guard holds and when the event never dispatched.
 */
function segmentRow(root: Root, id: string) {
  return root
    .findAllByType(SegmentRow)
    .find((n) => (n.props as { segment: Segment }).segment.id === id);
}

/** Drives the destructive button of the Nth confirm, found by STYLE not index. */
function acceptConfirm(spy: jest.SpyInstance, call = 0) {
  const buttons = spy.mock.calls[call][2] as { style?: string; onPress?: () => void }[];
  const destructive = buttons.find((b) => b.style === "destructive");
  expect(destructive).toBeDefined();
  act(() => destructive?.onPress?.());
}

describe("Segments — the collapsing header", () => {
  it("replaces BackHeader with a CollapsingHeader carrying the eyebrow and title", () => {
    const { getByTestId, getAllByText } = render(<SegmentsScreen />);
    expect(getByTestId("collapsing-header")).toBeTruthy();
    expect(getAllByText("Segments")).toHaveLength(2);
    expect(getAllByText("MARKETING")).toHaveLength(1);
  });

  it("keeps a back affordance, and it navigates back", () => {
    const { getByLabelText } = render(<SegmentsScreen />);
    fireEvent.press(getByLabelText("Go back"));
    expect(mockBack).toHaveBeenCalledTimes(1);
  });

  it("gives the chevron its own nav row rather than indenting the title", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    expect(getByTestId("collapsing-header-nav-row")).toBeTruthy();
  });

  it("keeps the New segment action in the trailing slot", () => {
    const { getByLabelText } = render(<SegmentsScreen />);
    fireEvent.press(getByLabelText("New segment"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/segments/new");
  });

  it("drives the header from an animated list, throttled at 16ms", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    const list = getByTestId("segments-list");
    expect(list.props.onScroll).toEqual(expect.any(Function));
    expect(list.props.scrollEventThrottle).toBe(16);
  });

  /**
   * The one list screen in the app with NO chip strip, and that is a decision
   * rather than an omission: a segment has no status, no lifecycle and no
   * axis to filter on, so a strip here could only ever hold a single "All"
   * pill — 40pt of chrome that filters nothing.
   */
  it("renders no filter chips at all — the resource has no status axis", () => {
    const { queryByTestId } = render(<SegmentsScreen />);
    expect(queryByTestId("filter-chips-block")).toBeNull();
  });
});

describe("Segments — the long-press menu", () => {
  const KEYS = ["edit", "delete"];

  it("renders exactly two items", () => {
    const { getByTestId, getAllByTestId, queryAllByTestId } = render(<SegmentsScreen />);
    for (const id of ["seg-summer", "seg-lapsed"]) {
      longPress(getByTestId, id);
      expect(getAllByTestId(/^action-sheet-item-/)).toHaveLength(2);
      for (const key of KEYS) expect(getByTestId(`action-sheet-item-${key}`)).toBeTruthy();
      fireEvent.press(getByTestId("action-sheet-item-edit"));
      expect(queryAllByTestId(/^action-sheet-item-/)).toHaveLength(0);
    }
  });

  it("titles the sheet with the segment's own name", () => {
    const { getByTestId, getAllByText } = render(<SegmentsScreen />);
    const rowOnly = getAllByText("Summer buyers").length;
    longPress(getByTestId, "seg-summer");
    expect(getAllByText("Summer buyers")).toHaveLength(rowOnly + 1);
  });

  it("opens the segment detail from Edit", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-lapsed");
    fireEvent.press(getByTestId("action-sheet-item-edit"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/segments/seg-lapsed");
  });

  it("offers no Duplicate item (no endpoint exists)", () => {
    const { getByTestId, getAllByTestId, queryByText } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-summer");
    const keys = getAllByTestId(/^action-sheet-item-/).map(
      (el) => (el.props as { testID: string }).testID,
    );
    expect(keys).toEqual(["action-sheet-item-edit", "action-sheet-item-delete"]);
    expect(queryByText(/duplicate/i)).toBeNull();
  });

  /**
   * Never disabled — and that is the point. The list carries no
   * campaign-linkage field, so a client-side gate could only be a guess; the
   * server holds the fact and refuses with a 409 that names the count. A
   * greyed-out Delete here would be wrong roughly as often as it was right.
   */
  it("leaves Delete enabled on every segment — only the server knows", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    for (const id of ["seg-summer", "seg-lapsed"]) {
      longPress(getByTestId, id);
      expect(getByTestId("action-sheet-item-delete").props.accessibilityState.disabled).toBe(
        false,
      );
      fireEvent.press(getByTestId("action-sheet-item-edit"));
    }
  });

  it("paints Delete as the destructive item, not merely labels it", () => {
    const { getByText, getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-summer");
    expect(StyleSheet.flatten(getByText("Delete").props.style).color).toBe(theme.colors.danger);
    expect(StyleSheet.flatten(getByText("Edit").props.style)?.color).not.toBe(
      theme.colors.danger,
    );
  });
});

describe("Segments — Delete is confirmed before it fires", () => {
  let alertSpy: jest.SpyInstance;
  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  it("does not fire the mutation until the confirm is accepted", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-summer");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("names the segment in the confirm and states the delete is permanent", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-summer");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    expect(alertSpy.mock.calls[0][0]).toBe("Delete segment?");
    expect(alertSpy.mock.calls[0][1]).toContain("Summer buyers");
    expect(alertSpy.mock.calls[0][1]).toContain("permanently deleted");
    expect(alertSpy.mock.calls[0][1]).toContain("cannot be undone");
  });

  it("offers a Cancel with no handler at all", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-summer");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    const buttons = alertSpy.mock.calls[0][2] as { text: string; onPress?: () => void }[];
    const cancel = buttons.find((b) => b.text === "Cancel");
    expect(cancel).toBeDefined();
    expect(cancel?.onPress).toBeUndefined();
    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("sends the delete with just the segment id once accepted", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-lapsed");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(mockDelete).toHaveBeenCalledTimes(1);
    expect(mockDelete.mock.calls[0][0]).toBe("seg-lapsed");
  });

  // No optimistic hide: `useDeleteSegment` invalidates ["segments"] AND
  // ["campaigns"], so the refetch is authoritative about the list's contents.
  it("leaves the row on screen until the refetch removes it", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    longPress(getByTestId, "seg-lapsed");
    fireEvent.press(getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(getByTestId("segment-row-seg-lapsed")).toBeTruthy();
  });
});

describe("Segments — no swipe", () => {
  it("mounts no SwipeRow on any row", () => {
    const { UNSAFE_root } = render(<SegmentsScreen />);
    expect(swipeRows(UNSAFE_root)).toHaveLength(0);
  });
});

describe("Segments — in-flight rows", () => {
  let alertSpy: jest.SpyInstance;
  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  function fireDelete(root: ReturnType<typeof render>, id: string) {
    longPress(root.getByTestId, id);
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy, alertSpy.mock.calls.length - 1);
  }

  it("suppresses long-press while that segment's own request is open", () => {
    const root = render(<SegmentsScreen />);
    fireDelete(root, "seg-lapsed");
    expect(segmentRow(root.UNSAFE_root, "seg-lapsed")?.props.onLongPress).toBeUndefined();
    expect(segmentRow(root.UNSAFE_root, "seg-summer")?.props.onLongPress).toBeDefined();
  });

  it("re-arms the row once the mutation settles", () => {
    const root = render(<SegmentsScreen />);
    fireDelete(root, "seg-lapsed");
    act(() => (mockDelete.mock.calls[0][1].onSuccess as () => void)());
    expect(segmentRow(root.UNSAFE_root, "seg-lapsed")?.props.onLongPress).toBeDefined();
  });

  it("releases the row on a refusal too, so the merchant can act on the reason", () => {
    const root = render(<SegmentsScreen />);
    fireDelete(root, "seg-summer");
    act(() => (mockDelete.mock.calls[0][1].onError as (e?: unknown) => void)(new Error("x")));
    expect(segmentRow(root.UNSAFE_root, "seg-summer")?.props.onLongPress).toBeDefined();
  });

  it("fires the success and failure haptics exactly once each", () => {
    const root = render(<SegmentsScreen />);
    fireDelete(root, "seg-lapsed");
    act(() => (mockDelete.mock.calls[0][1].onSuccess as () => void)());
    expect(adminHaptics.actionSucceeded).toHaveBeenCalledTimes(1);

    fireDelete(root, "seg-summer");
    act(() => (mockDelete.mock.calls[1][1].onError as (e?: unknown) => void)(new Error("x")));
    expect(adminHaptics.actionFailed).toHaveBeenCalledTimes(1);
  });
});

/**
 * The refusal, surfaced.
 *
 * `SegmentRow` carries no badge and the list never hides a row optimistically,
 * so a REFUSED delete and a delete still in flight render identically. The
 * server's own sentence is the only thing that tells them apart — and it is
 * the only place the blocking campaign count exists, so it is passed through
 * VERBATIM rather than paraphrased into generic copy.
 */
describe("Segments — surfacing a refused delete", () => {
  let alertSpy: jest.SpyInstance;
  beforeEach(() => {
    alertSpy = jest.spyOn(Alert, "alert").mockImplementation(() => {});
  });
  afterEach(() => alertSpy.mockRestore());

  function apiError(status: number, code: string, message: string): Error {
    const err = new Error(message) as Error & { status: number; code: string };
    err.name = "ApiError";
    err.status = status;
    err.code = code;
    return err;
  }

  function failDelete(root: ReturnType<typeof render>, error: unknown) {
    longPress(root.getByTestId, "seg-summer");
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy, alertSpy.mock.calls.length - 1);
    act(() => (mockDelete.mock.calls.at(-1)?.[1].onError as (e: unknown) => void)(error));
  }

  it("says nothing until something fails", () => {
    const root = render(<SegmentsScreen />);
    longPress(root.getByTestId, "seg-lapsed");
    fireEvent.press(root.getByTestId("action-sheet-item-delete"));
    acceptConfirm(alertSpy);
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  it("names what the merchant tried", () => {
    const root = render(<SegmentsScreen />);
    failDelete(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-title")).toHaveTextContent(
      "Couldn't delete this segment",
    );
  });

  /**
   * The COUNT is the whole point. It exists only in the server's sentence
   * (`apperrors.SegmentInUse`) — the client has no campaign-linkage field to
   * re-derive it from — so generic copy here would throw away the one part
   * the merchant can act on.
   */
  it("surfaces the pre-check's counted reason verbatim on 409 segment_in_use", () => {
    const root = render(<SegmentsScreen />);
    failDelete(
      root,
      apiError(
        409,
        "segment_in_use",
        "segment is still used by 1 campaign and cannot be deleted",
      ),
    );
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(
      // The WHOLE sentence, not a substring: "verbatim" is the property
      // under test, and a substring match would pass against copy that
      // wrapped the server's words in invented framing.
      "segment is still used by 1 campaign and cannot be deleted.",
    );
  });

  it("pluralises from the server, never from the client", () => {
    const root = render(<SegmentsScreen />);
    failDelete(
      root,
      apiError(
        409,
        "segment_in_use",
        "segment is still used by 2 campaigns and cannot be deleted",
      ),
    );
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(
      "segment is still used by 2 campaigns and cannot be deleted.",
    );
  });

  /**
   * The SECOND server path, and it is not the same sentence: the Postgres FK
   * translation has no count to report and says "at least one campaign"
   * instead (campaign/repository.go:125-141). Both are merchant-readable and
   * both must reach the screen unaltered — a client that only handled the
   * counted form would fall back to generic copy for the TOCTOU race.
   */
  it("surfaces the FK path's uncounted reason verbatim too", () => {
    const root = render(<SegmentsScreen />);
    failDelete(
      root,
      apiError(
        409,
        "segment_in_use",
        "segment is still used by at least one campaign and cannot be deleted",
      ),
    );
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(
      "segment is still used by at least one campaign and cannot be deleted.",
    );
  });

  it("distinguishes an unreachable server from a refusal", () => {
    const root = render(<SegmentsScreen />);
    failDelete(root, new TypeError("Network request failed"));
    expect(root.getByTestId("action-failure-detail")).toHaveTextContent(/reach the server/i);
  });

  it("keeps two failures down to one readable message", () => {
    const root = render(<SegmentsScreen />);
    failDelete(root, new Error("x"));
    failDelete(root, new Error("x"));
    expect(root.getAllByTestId("action-failure-notice")).toHaveLength(1);
  });

  it("clears itself on dismiss", () => {
    const root = render(<SegmentsScreen />);
    failDelete(root, new Error("x"));
    fireEvent.press(root.getByTestId("action-failure-dismiss"));
    expect(root.queryByTestId("action-failure-notice")).toBeNull();
  });

  // The segment is still there afterwards — nothing was destroyed, and the
  // merchant's next move is to retarget the campaign, not to hunt for a row.
  it("leaves the refused segment in the list", () => {
    const root = render(<SegmentsScreen />);
    failDelete(root, apiError(409, "segment_in_use", "segment is still used by 1 campaign and cannot be deleted"));
    expect(root.getByTestId("segment-row-seg-summer")).toBeTruthy();
  });
});

describe("Segments — list states", () => {
  it("opens the segment on tap", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    fireEvent.press(getByTestId("segment-row-seg-summer"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/marketing/segments/seg-summer");
  });

  it("tells a screen-reader user the row has more actions", () => {
    const { getByTestId } = render(<SegmentsScreen />);
    expect(getByTestId("segment-row-seg-summer").props.accessibilityHint).toBe(
      "Long press for more actions",
    );
  });

  it("shows a left-aligned empty state when there are no segments", () => {
    mockState.segments = [];
    const { getByText } = render(<SegmentsScreen />);
    expect(getByText("No segments yet")).toBeTruthy();
  });

  it("offers a retry when the list fails to load", () => {
    mockState.segments = [];
    mockState.isError = true;
    const { getByText } = render(<SegmentsScreen />);
    fireEvent.press(getByText("Try again"));
    expect(mockRefetch).toHaveBeenCalledTimes(1);
  });
});
