import { describe, it, expect } from "vitest";
import {
  toCreateBody,
  SupportConversationSchema,
  SupportMessageSchema,
  QueueStateSchema,
} from "../types";

describe("toCreateBody", () => {
  it("maps friendly input to otto's wire shape with defaults", () => {
    expect(toCreateBody({ message: "hi", reason: "billing", statusInfo: "late" })).toEqual({
      message: "hi",
      reason: "billing",
      status_info: "late",
      subject: "",
      name: "",
      email: "",
      dob: "",
    });
  });

  it("passes through optional fields", () => {
    const out = toCreateBody({
      message: "m",
      reason: "order_issue",
      statusInfo: "s",
      subject: "Order #5",
      name: "Sam",
      email: "s@x.com",
      dob: "1990-01-01",
    });
    expect(out.subject).toBe("Order #5");
    expect(out.dob).toBe("1990-01-01");
  });
});

describe("schemas", () => {
  it("parses a conversation, keeping unknown fields (passthrough)", () => {
    const c = SupportConversationSchema.parse({
      id: "c1",
      status: "active",
      unread_count_customer: 2,
    });
    expect(c.id).toBe("c1");
    expect((c as Record<string, unknown>).unread_count_customer).toBe(2);
    expect(c.case_id).toBe(""); // default applied
  });

  it("rejects an invalid status", () => {
    expect(() => SupportConversationSchema.parse({ id: "c1", status: "frozen" })).toThrow();
  });

  it("parses a message", () => {
    const m = SupportMessageSchema.parse({
      id: "m1",
      sender_type: "system",
      body: "auto-closed",
      created_at: "2026-06-21T10:00:00Z",
    });
    expect(m.sender_name).toBe("");
  });

  it("applies queue defaults", () => {
    const q = QueueStateSchema.parse({ status: "pending" });
    expect(q.position).toBe(0);
    expect(q.all_busy).toBe(false);
  });
});
