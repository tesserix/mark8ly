import {
  customerSchema,
  customerListSchema,
  customerDetailSchema,
} from "@repo/mobile-shared/api/schemas/customers";

// The exact payload GET /customers returned from prod on 2026-07-16.
const REAL_LIST = {
  data: [
    {
      id: "8b52db9e-5dcf-40e0-b81d-ea5d7bcc3152",
      email: "mahesh.sangawar@gmail.com",
      tags: [],
      status: "active",
      marketing_opt_in: false,
      order_count: 0,
      total_spent: 0,
      created_at: "2026-05-06T02:27:38Z",
      updated_at: "2026-05-18T00:24:50Z",
    },
  ],
  meta: { page: 1, page_size: 50, total: 1, total_pages: 1 },
};

describe("customerSchema", () => {
  it("accepts the real customer, which has NO first_name/last_name/phone", () => {
    const parsed = customerListSchema.parse(REAL_LIST);
    expect(parsed.data[0]!.email).toBe("mahesh.sangawar@gmail.com");
    expect(parsed.data[0]!.first_name).toBeUndefined();
    expect(parsed.meta.total).toBe(1);
  });

  it("accepts names when present", () => {
    const parsed = customerSchema.parse({
      ...REAL_LIST.data[0],
      first_name: "Maya",
      last_name: "Chen",
      phone: "+61 400 111 222",
    });
    expect(parsed.first_name).toBe("Maya");
  });

  it("coerces total_spent from a quoted decimal string", () => {
    const parsed = customerSchema.parse({ ...REAL_LIST.data[0], total_spent: "48.20" });
    expect(parsed.total_spent).toBe(48.2);
  });

  it("rejects a payload missing a required field, naming the path", () => {
    const bad = { ...REAL_LIST.data[0] } as Record<string, unknown>;
    delete bad.email;
    const res = customerSchema.safeParse(bad);
    expect(res.success).toBe(false);
    if (!res.success) expect(res.error.issues[0]!.path).toContain("email");
  });

  it("detail is flat with addresses, NOT wrapped in {data}", () => {
    const parsed = customerDetailSchema.parse({ ...REAL_LIST.data[0], addresses: [] });
    expect(parsed.addresses).toEqual([]);
    expect(parsed.id).toBe("8b52db9e-5dcf-40e0-b81d-ea5d7bcc3152");
  });

  it("detail accepts a populated address", () => {
    const parsed = customerDetailSchema.parse({
      ...REAL_LIST.data[0],
      addresses: [
        {
          id: "a-1",
          is_default: true,
          name: "Maya Chen",
          line1: "12 Campbell Pde",
          city: "Bondi Beach",
          country_code: "AU",
        },
      ],
    });
    expect(parsed.addresses[0]!.line1).toBe("12 Campbell Pde");
  });
});
