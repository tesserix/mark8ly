import {
  couponSchema,
  couponListSchema,
  couponDetailEnvelopeSchema,
} from "@repo/mobile-shared/api/schemas/coupons";
import { campaignSchema, campaignListSchema } from "@repo/mobile-shared/api/schemas/campaigns";
import { segmentSchema, segmentListSchema } from "@repo/mobile-shared/api/schemas/segments";
import {
  giftCardDetailSchema,
  giftCardListSchema,
} from "@repo/mobile-shared/api/schemas/gift-cards";
import {
  loyaltyProgramSchema,
  loyaltyMemberListSchema,
  loyaltyMemberDetailSchema,
} from "@repo/mobile-shared/api/schemas/loyalty";

describe("couponSchema", () => {
  const FULL = {
    id: "c1",
    code: "SAVE10",
    title: "10% off",
    description: null,
    type: "percentage",
    value: "10",
    currency_code: null,
    min_purchase: null,
    max_discount: null,
    usage_limit: null,
    per_customer: 0,
    target_type: "all",
    target_ids: [],
    stackable: false,
    starts_at: "2026-07-01T00:00:00Z",
    ends_at: null,
    status: "active",
    usage_count: 0,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };
  it("parses with nullable (not absent) description/currency/decimals — no-omitempty pointers marshal null", () => {
    const c = couponSchema.parse(FULL);
    expect(c.description).toBeNull();
    expect(c.min_purchase).toBeNull();
    expect(c.value).toBe(10); // quoted decimal → number
  });
  it("parses decimals when present as quoted strings", () => {
    const c = couponSchema.parse({ ...FULL, min_purchase: "50.00", value: "199" });
    expect(c.min_purchase).toBe(50);
    expect(c.value).toBe(199);
  });
  it("list envelope is {data,total,page} (NOT {data,meta})", () => {
    const r = couponListSchema.parse({ data: [FULL], total: 1, page: 1 });
    expect(r.total).toBe(1);
  });
  it("detail envelope tolerates missing usage/usage_total (non-fatal path)", () => {
    expect(() => couponDetailEnvelopeSchema.parse({ data: FULL })).not.toThrow();
    const withUsage = couponDetailEnvelopeSchema.parse({ data: FULL, usage: [], usage_total: 0 });
    expect(withUsage.usage_total).toBe(0);
  });
});

describe("campaignSchema", () => {
  const FULL = {
    id: "cm1",
    name: "Summer blast",
    type: "email",
    status: "draft",
    subject: null,
    content: null,
    segment_id: null,
    coupon_id: null,
    scheduled_at: null,
    sent_at: null,
    total_recipients: 0,
    delivered: 0,
    opened: 0,
    clicked: 0,
    converted: 0,
    unsubscribed: 0,
    failed: 0,
    revenue: "12.50",
    show_on_storefront: false,
    storefront_priority: 0,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };
  it("nullable subject/content + omitempty storefront_label absent; revenue string→number", () => {
    const c = campaignSchema.parse(FULL);
    expect(c.subject).toBeNull();
    expect(c.storefront_label).toBeUndefined();
    expect(c.revenue).toBe(12.5);
  });
  it("list envelope is standard {data,meta}", () => {
    const r = campaignListSchema.parse({
      data: [FULL],
      meta: { page: 1, page_size: 20, total: 1, total_pages: 1 },
    });
    expect(r.meta.total_pages).toBe(1);
  });
});

describe("segmentSchema", () => {
  it("nullable description; rules is opaque string; list is bare {data} (no meta)", () => {
    const seg = {
      id: "s1",
      name: "VIPs",
      description: null,
      rules: '[{"field":"total_spent","op":"gt","value":100}]',
      member_count: 3,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
    };
    expect(segmentSchema.parse(seg).description).toBeNull();
    expect(() => segmentListSchema.parse({ data: [seg] })).not.toThrow();
  });
});

describe("giftCardDetailSchema", () => {
  it("omitempty optionals absent; created_at present; detail carries transactions[]", () => {
    const detail = {
      id: "g1",
      code: "GC-XXXX",
      code_display: "GC-••••",
      initial_balance: "100.00",
      current_balance: "80.00",
      currency_code: "AUD",
      status: "active",
      created_at: "2026-07-01T00:00:00Z",
      purchased_via_storefront: false,
      transactions: [
        {
          id: "t1",
          type: "redeem",
          amount: "20.00",
          balance_after: "80.00",
          created_at: "2026-07-02T00:00:00Z",
        },
      ],
    };
    const d = giftCardDetailSchema.parse(detail);
    expect(d.sender_name).toBeUndefined();
    expect(d.current_balance).toBe(80);
    expect(d.transactions[0]!.amount).toBe(20);
  });
  it("list is standard {data,meta}", () => {
    expect(() =>
      giftCardListSchema.parse({ data: [], meta: { page: 1, page_size: 20, total: 0, total_pages: 0 } }),
    ).not.toThrow();
  });
});

describe("loyalty schemas", () => {
  const PROGRAM = {
    id: "lp1",
    is_active: true,
    points_per_unit: "1",
    points_currency: "AUD",
    signup_bonus: 100,
    referral_bonus: 50,
    referee_bonus: 25,
    point_expiry_days: null,
    min_redeem_points: 500,
    points_value: "0.01",
    tiers: [{ name: "Gold", min_points: 1000, multiplier: "1.5" }],
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
  };
  it("program: nullable point_expiry_days; decimal multiplier/points → number", () => {
    const p = loyaltyProgramSchema.parse(PROGRAM);
    expect(p.point_expiry_days).toBeNull();
    expect(p.tiers[0]!.multiplier).toBe(1.5);
    expect(p.points_value).toBe(0.01);
  });
  it("member list uses loyalty meta {total,page,limit}", () => {
    const r = loyaltyMemberListSchema.parse({ data: [], meta: { total: 0, page: 1, limit: 20 } });
    expect(r.meta.limit).toBe(20);
  });
  it("member detail nests transactions:{data,meta}", () => {
    const detail = {
      data: {
        id: "m1",
        customer_email: "a@b.com",
        points_balance: 120,
        lifetime_points: 300,
        tier: "Silver",
        referral_code: "REF123",
        enrolled_at: "2026-07-01T00:00:00Z",
      },
      transactions: { data: [], meta: { total: 0, page: 1, limit: 20 } },
    };
    expect(() => loyaltyMemberDetailSchema.parse(detail)).not.toThrow();
  });
});
