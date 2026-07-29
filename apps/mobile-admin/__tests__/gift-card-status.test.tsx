// Gift-card Enable/Disable — the wire, the cache, and the legality rule.
//
// Three things are pinned here, all of them invisible in a screenshot:
//
//  1. WHICH URLs. The mobile client can only reach
//     `/api/v1/mobile/admin/stores/{id}/…`, and Disable/Enable originally
//     shipped on the WEB admin route table only — from the phone that is a
//     gin-level 404 the handler never sees. The Go side of that gap is
//     pinned in `mobile_routes_giftcards_test.go`; this is the client half,
//     so a path typo here can't quietly re-open it.
//  2. WHICH cache keys go stale. The screen does no optimistic update, so a
//     missed invalidation means the badge never flips — the merchant taps
//     Disable, the request succeeds, and the row keeps saying Active.
//  3. WHICH cards may be toggled at all. The backend accepts exactly two
//     target states and refuses every other source state; the screen's whole
//     gesture gate is this one predicate, so it is tested as a table rather
//     than through the UI.
import React from "react";
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createGiftCardsApi } from "@repo/mobile-shared/api/gift-cards";
import { useSetGiftCardStatus } from "@/lib/admin-api/gift-card-actions";
import { canSetGiftCardStatus } from "@/lib/gift-card-status";
import type { GiftCard } from "@repo/mobile-shared/api/types";

type Call = { method: string; path: string; body?: unknown; schema?: unknown };

const calls: Call[] = [];

/**
 * A recording stand-in for the store-scoped client. `post` returns the
 * enveloped shape the api layer unwraps with `.then(r => r.data)`.
 *
 * The SAME fake serves the route assertions and the invalidation ones, so
 * the hook test exercises the real `createGiftCardsApi` rather than a
 * hand-written duplicate of it — a path typo cannot pass one and fail the
 * other.
 */
function fakeClient() {
  const stub = () => Promise.resolve({ data: { id: "g1", status: "disabled" } });
  return {
    get: (path: string) => {
      calls.push({ method: "GET", path });
      return stub();
    },
    post: (path: string, body?: unknown, schema?: unknown) => {
      calls.push({ method: "POST", path, body, schema });
      return stub();
    },
  } as unknown as Parameters<typeof createGiftCardsApi>[0];
}

// `mock`-prefixed so jest's out-of-scope-variable guard allows the factory
// below to close over it.
const mockClient = fakeClient();
jest.mock("@/lib/api-client", () => ({ useApiClient: () => mockClient }));

let queryClient: QueryClient;

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  calls.length = 0;
  queryClient = new QueryClient({
    defaultOptions: {
      // gcTime 0 so no cache-eviction timer outlives the test and holds the
      // jest worker open.
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
});

afterEach(() => {
  queryClient.clear();
});

function card(over: Partial<GiftCard> = {}): GiftCard {
  return {
    id: "g1",
    code: "GIFTABCD1234",
    code_display: "GIFT-••••-1234",
    status: "active",
    initial_balance: 10000,
    current_balance: 7500,
    currency_code: "AUD",
    created_at: "2026-07-01T00:00:00Z",
    purchased_via_storefront: false,
    ...over,
  } as GiftCard;
}

describe("createGiftCardsApi — the status routes", () => {
  it("POSTs to /gift-cards/:id/disable and /enable with NO body", async () => {
    const api = createGiftCardsApi(mockClient);
    await api.disable("g1");
    await api.enable("g1");

    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "POST /gift-cards/g1/disable",
      "POST /gift-cards/g1/enable",
    ]);
    // The endpoints take no request body (gift_cards.go setStatus reads only
    // the path params). Sending one would be harmless today and a silent
    // contract lie the moment the handler starts binding.
    for (const c of calls) expect(c.body).toBeUndefined();
  });

  it("parses both responses through a schema, so a contract break is loud", async () => {
    const api = createGiftCardsApi(mockClient);
    await api.disable("g1");
    await api.enable("g1");
    // Not "some schema": a missing third argument means the response is
    // returned unvalidated and a renamed field reaches the row as undefined.
    for (const c of calls) expect(c.schema).toBeDefined();
  });
});

describe("useSetGiftCardStatus", () => {
  it("routes an 'active' target to enable and a 'disabled' target to disable", async () => {
    const { result } = renderHook(() => useSetGiftCardStatus(), { wrapper });

    result.current.mutate({ id: "g7", status: "active" });
    await waitFor(() => expect(calls).toHaveLength(1));
    expect(calls[0].path).toBe("/gift-cards/g7/enable");

    result.current.mutate({ id: "g7", status: "disabled" });
    await waitFor(() => expect(calls).toHaveLength(2));
    expect(calls[1].path).toBe("/gift-cards/g7/disable");
  });

  it("makes BOTH the list and that card's detail stale on success", async () => {
    // Behavioural, not key-shape: `["gift-cards"]` prefix-matches both of
    // these already, and asserting the literal key would pass just as well
    // against an invalidation that matched neither.
    //
    // Seeded with `setQueryData` rather than through a mounted `useQuery`:
    // an ACTIVE observer refetches the instant it is invalidated and clears
    // `isInvalidated` again, so the assertion would race its own refetch.
    // These are the exact keys `use-gift-cards.ts` builds.
    //
    // The suite-wide `gcTime: 0` would evict an observer-less entry the
    // instant it is created, so this client keeps them alive; `clear()` in
    // `afterEach` destroys the entries and their timers with them, so no
    // timer outlives the test.
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, gcTime: 60_000 },
        mutations: { retry: false, gcTime: 0 },
      },
    });
    queryClient.setQueryData(["gift-cards", "list", undefined], { pages: [] });
    queryClient.setQueryData(["gift-cards", "detail", "g7"], { id: "g7" });

    const { result } = renderHook(() => useSetGiftCardStatus(), { wrapper });
    result.current.mutate({ id: "g7", status: "disabled" });

    await waitFor(() => {
      expect(queryClient.getQueryState(["gift-cards", "list", undefined])?.isInvalidated).toBe(
        true,
      );
      expect(queryClient.getQueryState(["gift-cards", "detail", "g7"])?.isInvalidated).toBe(true);
    });
  });

  it("does NOT invalidate the dashboard — its payload has no gift-card field", async () => {
    const invalidate = jest.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useSetGiftCardStatus(), { wrapper });
    result.current.mutate({ id: "g7", status: "disabled" });
    await waitFor(() => expect(invalidate).toHaveBeenCalled());

    const keys = invalidate.mock.calls.map((c) => JSON.stringify(c[0]?.queryKey));
    expect(keys).not.toContain(JSON.stringify(["dashboard"]));
  });
});

describe("canSetGiftCardStatus", () => {
  // The reference instant every expiry case below is judged against, so
  // "expired" never means "whenever this suite happens to run".
  const NOW = Date.parse("2026-07-29T00:00:00Z");
  const PAST = "2026-07-28T00:00:00Z";
  const FUTURE = "2026-08-28T00:00:00Z";

  it("allows disabling an active card and enabling a disabled one", () => {
    expect(canSetGiftCardStatus(card({ status: "active" }), "disabled", NOW)).toBe(true);
    expect(canSetGiftCardStatus(card({ status: "disabled" }), "active", NOW)).toBe(true);
  });

  it("refuses the state the card is already in", () => {
    // The server answers this with an idempotent 200, NOT a 409 — so this
    // gate is about the merchant, not the wire. Offering "Enable" on a card
    // that is already active is an action that visibly does nothing.
    expect(canSetGiftCardStatus(card({ status: "active" }), "active", NOW)).toBe(false);
    expect(canSetGiftCardStatus(card({ status: "disabled" }), "disabled", NOW)).toBe(false);
  });

  it.each(["pending", "depleted", "refunded"])(
    "refuses BOTH directions for a %s card (server: 409 invalid_transition)",
    (status) => {
      expect(canSetGiftCardStatus(card({ status }), "active", NOW)).toBe(false);
      expect(canSetGiftCardStatus(card({ status }), "disabled", NOW)).toBe(false);
    },
  );

  it("refuses BOTH directions for an EXPIRED card, whatever its status says", () => {
    // The trap: there is no `expired` status in the backend enum
    // (giftcard/models.go: pending|active|disabled|depleted|refunded).
    // Expiry is a TIMESTAMP, so an expired card still reads `status:
    // "active"` — a gate that switched on status alone would arm both
    // gestures on it and collect a 410 every time.
    expect(canSetGiftCardStatus(card({ status: "active", expires_at: PAST }), "disabled", NOW)).toBe(
      false,
    );
    expect(
      canSetGiftCardStatus(card({ status: "disabled", expires_at: PAST }), "active", NOW),
    ).toBe(false);
  });

  it("leaves a card with a FUTURE expiry fully toggleable", () => {
    expect(
      canSetGiftCardStatus(card({ status: "active", expires_at: FUTURE }), "disabled", NOW),
    ).toBe(true);
  });

  it("treats an unparseable expires_at as not-expired rather than locking the card", () => {
    // Fail open on garbage: the server is authoritative and will refuse if
    // it really is expired. Failing closed would strand a perfectly good
    // card behind a gesture the merchant cannot reach and cannot diagnose.
    expect(
      canSetGiftCardStatus(card({ status: "active", expires_at: "not-a-date" }), "disabled", NOW),
    ).toBe(true);
  });
});
