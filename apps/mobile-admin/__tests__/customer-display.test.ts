import { customerStatusLabel, customerStatusTone } from "@/lib/customer-display";

describe("customerStatusLabel", () => {
  it("titleises the two statuses the backend actually returns", () => {
    expect(customerStatusLabel("active")).toBe("Active");
    expect(customerStatusLabel("blocked")).toBe("Blocked");
  });

  // The wire schema types `status` as a bare `z.string()` on purpose — a
  // server that adds a third status must not blank the address book — so
  // this has to cope with a value it has never seen. Plain titleising
  // rendered the wire token verbatim ("pending_review") on screen and to
  // VoiceOver; same defect `productStatusLabel` had to close.
  it("humanises a status it has never seen instead of leaking the wire token", () => {
    expect(customerStatusLabel("pending_review")).toBe("Pending review");
    expect(customerStatusLabel("under-review")).toBe("Under review");
  });

  it("never announces an empty badge", () => {
    expect(customerStatusLabel("")).toBe("Unknown");
  });
});

describe("customerStatusTone", () => {
  it("tints blocked as danger, matching every other restrictive state in the app", () => {
    expect(customerStatusTone("blocked")).toBe("danger");
  });

  it("tints anything unrecognised as muted rather than borrowing the danger alarm", () => {
    expect(customerStatusTone("pending_review")).toBe("muted");
    expect(customerStatusTone("active")).toBe("muted");
  });
});
