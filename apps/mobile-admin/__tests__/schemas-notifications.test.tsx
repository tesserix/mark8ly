import {
  notificationSchema,
  notificationListSchema,
} from "@repo/mobile-shared/api/schemas/notifications";

// The EXACT payload GET /notifications returned from prod on 2026-07-16.
const REAL_EMPTY = { notifications: [], page: 1, per_page: 20, total: 0 };

const ITEM = {
  id: "n-1",
  type: "new_order",
  title: "New order #1001",
  message: "Maya Chen placed an order",
  resource_type: "order",
  resource_id: "o-1001",
  is_read: false,
  created_at: "2026-07-14T09:00:00Z",
};

describe("notificationListSchema", () => {
  it("parses the real {notifications, page, per_page, total} envelope", () => {
    const parsed = notificationListSchema.parse(REAL_EMPTY);
    expect(parsed.notifications).toEqual([]);
    expect(parsed.total).toBe(0);
    expect(parsed.per_page).toBe(20);
  });

  it("rejects the {data, meta} envelope — notifications is NOT paginated()", () => {
    expect(
      notificationListSchema.safeParse({
        data: [],
        meta: { page: 1, page_size: 20, total: 0, total_pages: 0 },
      }).success,
    ).toBe(false);
  });

  it("rejects the {items} fiction", () => {
    expect(notificationListSchema.safeParse({ items: [], total: 0 }).success).toBe(false);
  });

  it("parses a populated notification", () => {
    const parsed = notificationListSchema.parse({ ...REAL_EMPTY, notifications: [ITEM], total: 1 });
    expect(parsed.notifications[0]!.is_read).toBe(false);
    expect(parsed.notifications[0]!.title).toBe("New order #1001");
  });

  it("accepts a notification with no message (omitempty)", () => {
    const bare = { ...ITEM } as Record<string, unknown>;
    delete bare.message;
    delete bare.resource_type;
    delete bare.resource_id;
    const parsed = notificationSchema.parse(bare);
    expect(parsed.message).toBeUndefined();
  });

  it("names the field path when is_read is the wrong type", () => {
    const res = notificationSchema.safeParse({ ...ITEM, is_read: "false" });
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("is_read");
  });
});
