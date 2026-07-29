// The copy a merchant reads when a mutation fails.
//
// Until now a failed activate/archive/block/close produced ONE failure
// haptic and nothing else: no message, no explanation, and on Customers
// (whose row carries no status badge under the default "All" filter) no
// visible change of any kind. Success and failure were indistinguishable.
//
// "Something went wrong" would not have been worth building. The server
// already returns structured, merchant-readable prose and several messages
// are load-bearing — `segment_in_use` names the number of campaigns still
// pointing at the segment, and nothing on the client knows that number. So
// the rule this file encodes is: PREFER the server's own sentence wherever
// it is readable, and never invent copy that contradicts it.
//
// The one distinction the client must draw for itself is "could not reach
// the server" vs "the server refused this" — the merchant's next action
// differs (retry later / stop and look).
import { describeActionFailure } from "@/lib/action-failure-message";

/** A server error as `ApiError` puts it on the wire. Structural, on purpose. */
function apiError(status: number, code: string, message: string): Error {
  const err = new Error(message) as Error & { status: number; code: string };
  err.name = "ApiError";
  err.status = status;
  err.code = code;
  return err;
}

describe("describeActionFailure — the action is always named", () => {
  // The one thing every branch owes the merchant. A message that says only
  // "that didn't work" is undiagnosable on a phone with no console: the row
  // looks unchanged after the refetch either way.
  const cases: [string, unknown][] = [
    ["a network drop", new TypeError("Network request failed")],
    ["a timeout", apiError(408, "timeout", "The request timed out.")],
    ["a refusal", apiError(409, "invalid_transition", "cannot transition from open to closed")],
    ["a server fault", apiError(500, "internal", "boom")],
    ["a thrown string", "kaboom"],
    ["nothing at all", undefined],
  ];

  for (const [label, error] of cases) {
    it(`names the action after ${label}`, () => {
      expect(describeActionFailure(error, "archive this product").title).toBe(
        "Couldn't archive this product",
      );
    });
  }
});

describe("describeActionFailure — reach vs refusal", () => {
  // The distinction that changes what the merchant does next. Collapsing
  // these two into one "couldn't do that" is the failure mode this whole
  // task exists to remove.
  it("says the DEVICE could not reach the server when nothing came back", () => {
    const { detail } = describeActionFailure(
      new TypeError("Network request failed"),
      "approve this order",
    );
    expect(detail).toMatch(/couldn't reach the server/i);
    expect(detail).toMatch(/connection/i);
  });

  it("says the SERVER answered and refused when it did", () => {
    const { detail } = describeActionFailure(
      apiError(409, "conflict", ""),
      "approve this order",
    );
    expect(detail).not.toMatch(/reach the server/i);
    expect(detail).toMatch(/refused/i);
  });

  it("distinguishes a timeout from an unreachable server", () => {
    const reach = describeActionFailure(new TypeError("Network request failed"), "x").detail;
    const timeout = describeActionFailure(
      apiError(408, "timeout", "The request timed out. Check your connection and try again."),
      "x",
    ).detail;
    expect(timeout).not.toBe(reach);
    expect(timeout).toMatch(/too long/i);
  });

  // Anything that is not shaped like an ApiError never made it past the
  // transport — including a bare string or `undefined` thrown from a mock.
  it("treats a non-ApiError as a transport failure, not a refusal", () => {
    expect(describeActionFailure("kaboom", "x").detail).toMatch(/couldn't reach the server/i);
    expect(describeActionFailure(undefined, "x").detail).toMatch(/couldn't reach the server/i);
  });
});

describe("describeActionFailure — the server's own prose", () => {
  // The case the brief singles out: the count is a fact ONLY the server
  // holds, so paraphrasing it loses the one thing that makes the message
  // actionable.
  it("surfaces segment_in_use verbatim, campaign count and all", () => {
    const server = "segment is still used by 3 campaigns and cannot be deleted";
    const { detail } = describeActionFailure(
      apiError(409, "segment_in_use", server),
      "delete this segment",
    );
    expect(detail).toContain(server);
    expect(detail).toContain("3 campaigns");
  });

  it("surfaces campaign_not_draft verbatim", () => {
    const server = "only draft campaigns can be deleted";
    expect(
      describeActionFailure(apiError(409, "campaign_not_draft", server), "delete this campaign")
        .detail,
    ).toContain(server);
  });

  it("surfaces invalid_transition verbatim", () => {
    const server = "cannot transition from closed to closed";
    expect(
      describeActionFailure(apiError(409, "invalid_transition", server), "close this ticket").detail,
    ).toContain(server);
  });

  it("surfaces gift_card_expired verbatim", () => {
    const server = "gift card has expired";
    expect(
      describeActionFailure(apiError(410, "gift_card_expired", server), "redeem this gift card")
        .detail,
    ).toContain(server);
  });

  it("closes the server's sentence rather than leaving it hanging", () => {
    const { detail } = describeActionFailure(
      apiError(409, "segment_in_use", "segment is still used by 1 campaign and cannot be deleted"),
      "delete this segment",
    );
    expect(detail.endsWith(".")).toBe(true);
  });
});

describe("describeActionFailure — prose the merchant must never see", () => {
  // `res.statusText` is the client's own fallback when the body isn't JSON.
  // "Bad Request" is true and useless; it must not displace copy that tells
  // the merchant what to do.
  it("rejects a bare HTTP status text", () => {
    const { detail } = describeActionFailure(apiError(400, "unknown", "Bad Request"), "x");
    expect(detail).not.toContain("Bad Request");
  });

  it("rejects a bare error code", () => {
    const { detail } = describeActionFailure(
      apiError(409, "segment_in_use", "segment_in_use"),
      "delete this segment",
    );
    expect(detail).not.toBe("segment_in_use.");
    // …and still says something specific, because the CODE is mapped even
    // when the prose is not usable.
    expect(detail).toMatch(/campaign/i);
  });

  // A contract mismatch is OUR bug, raised as a 500 with a zod field path as
  // its message ("data.0.title: Required"). Showing that to a merchant is
  // worse than showing nothing.
  it("never leaks a contract-mismatch field path", () => {
    const { detail } = describeActionFailure(
      apiError(500, "contract_mismatch", "data.0.title: Required"),
      "save this product",
    );
    expect(detail).not.toContain("data.0.title");
    expect(detail).toMatch(/server/i);
  });

  it("never leaks a 5xx internal message", () => {
    const { detail } = describeActionFailure(
      apiError(503, "unknown", "upstream connect error or disconnect/reset before headers"),
      "x",
    );
    expect(detail).not.toContain("upstream");
  });

  it("does not surface a template leak", () => {
    const { detail } = describeActionFailure(
      apiError(400, "unknown", "could not apply {{ discount }} to this order"),
      "x",
    );
    expect(detail).not.toContain("{{");
  });
});

describe("describeActionFailure — status-shaped fallbacks", () => {
  it("explains a 403 as permission, not as a network problem", () => {
    const { detail } = describeActionFailure(apiError(403, "forbidden", "Forbidden"), "x");
    expect(detail).toMatch(/permission/i);
  });

  it("explains a 404 as gone, and offers the refresh that fixes the list", () => {
    const { detail } = describeActionFailure(apiError(404, "not_found", "Not Found"), "x");
    expect(detail).toMatch(/no longer exists/i);
    expect(detail).toMatch(/refresh/i);
  });

  it("explains a 401 as a session end, never as a refusal to retry", () => {
    const { detail } = describeActionFailure(apiError(401, "unauthorized", "Session expired"), "x");
    expect(detail).toMatch(/sign in/i);
  });

  it("explains a 429 as rate limiting", () => {
    const { detail } = describeActionFailure(apiError(429, "rate_limited", "Too Many Requests"), "x");
    expect(detail).toMatch(/wait a moment/i);
  });

  // 401 carries a client-synthesised "Access denied"/"Session expired". It is
  // grammatical prose, so a naive readability gate would surface it in place
  // of the copy that actually tells the merchant to sign in again.
  it("does not let the 401 placeholder prose displace the sign-in instruction", () => {
    expect(describeActionFailure(apiError(401, "unauthorized", "Access denied"), "x").detail).toMatch(
      /sign in/i,
    );
  });
});
