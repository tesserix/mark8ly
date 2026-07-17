import {
  reviewSchema,
  reviewListSchema,
  reviewStatusSchema,
  replyBodySchema,
} from "@repo/mobile-shared/api/schemas/reviews";

const REAL_REVIEW = {
  id: "r1",
  product_id: "p1",
  customer_name: "Ada Lovelace",
  customer_email: "ada@example.com",
  rating: 5,
  content: "Excellent quality, fast shipping.",
  status: "approved",
  verified_purchase: true,
  featured: false,
  helpful_count: 3,
  not_helpful_count: 0,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  media: [
    { id: "m1", url: "https://cdn/x.jpg", position: 0, media_type: "image" },
  ],
  replies: [
    {
      id: "rep1",
      author_type: "merchant",
      author_name: "Bondi Store",
      content: "Thanks Ada!",
      created_at: "2026-07-02T00:00:00Z",
    },
  ],
};

describe("reviewSchema", () => {
  it("parses a full review with media + replies", () => {
    const parsed = reviewSchema.parse(REAL_REVIEW);
    expect(parsed.rating).toBe(5);
    expect(parsed.media[0]!.media_type).toBe("image");
    expect(parsed.replies[0]!.author_type).toBe("merchant");
  });

  it("accepts omitted optionals (title, published_at, customer_profile_id) — Go omitempty is ABSENT not null", () => {
    // These are absent from REAL_REVIEW; parse must not require them.
    const parsed = reviewSchema.parse(REAL_REVIEW);
    expect(parsed.title).toBeUndefined();
    expect(parsed.published_at).toBeUndefined();
  });

  it("rejects a null optional (must be absent, not null)", () => {
    expect(() => reviewSchema.parse({ ...REAL_REVIEW, title: null })).toThrow();
  });

  it("requires media and replies to be arrays (backend always emits them)", () => {
    const { media, ...noMedia } = REAL_REVIEW;
    expect(() => reviewSchema.parse(noMedia)).toThrow();
  });

  it("constrains status to the enum", () => {
    expect(reviewStatusSchema.parse("pending")).toBe("pending");
    expect(() => reviewStatusSchema.parse("published")).toThrow();
  });
});

describe("reviewListSchema", () => {
  it("parses the {data, meta} list envelope", () => {
    const parsed = reviewListSchema.parse({
      data: [REAL_REVIEW],
      meta: { page: 1, page_size: 50, total: 1, total_pages: 1 },
    });
    expect(parsed.data).toHaveLength(1);
    expect(parsed.meta.total_pages).toBe(1);
  });
});

describe("replyBodySchema", () => {
  it("rejects empty and over-5000-char replies", () => {
    expect(() => replyBodySchema.parse({ content: "   " })).toThrow();
    expect(() => replyBodySchema.parse({ content: "x".repeat(5001) })).toThrow();
    expect(replyBodySchema.parse({ content: "ok" }).content).toBe("ok");
  });
});
