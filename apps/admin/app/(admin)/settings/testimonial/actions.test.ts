import { beforeEach, expect, it, vi } from "vitest";

const session = vi.fn();
const request = vi.fn();
vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/auth/serverSession", () => ({
  getServerSessionContext: () => session(),
}));
vi.mock("@/lib/api/server/platformInternal", () => ({
  platformInternalFetch: (...args: unknown[]) => request(...args),
}));
import { saveTestimonial } from "./actions";

function form() {
  const data = new FormData();
  data.set("author_name", "Seller");
  data.set("quote", "Easy to set up.");
  data.set("rating", "4");
  data.set("intent", "publish");
  data.set("consent", "on");
  data.set("store_id", "someone-elses-store");
  data.set("uid", "someone-else");
  return data;
}

beforeEach(() => {
  vi.clearAllMocks();
  session.mockResolvedValue({
    role: "owner",
    userId: "real-user",
    currentStore: { id: "real-store" },
  });
  request.mockResolvedValue(
    new Response(JSON.stringify({ data: { published: true } }), {
      status: 200,
    }),
  );
});

it("uses the authenticated store and identity, ignoring form identity", async () => {
  expect(await saveTestimonial({}, form())).toMatchObject({
    ok: true,
    published: true,
  });
  expect(request).toHaveBeenCalledWith(
    "/internal/stores/real-store/testimonial",
    expect.objectContaining({
      method: "PUT",
      headers: expect.objectContaining({ "X-User-ID": "real-user" }),
    }),
  );
  const sent = JSON.parse(request.mock.calls[0][1].body);
  expect(sent).toEqual({
    author_name: "Seller",
    quote: "Easy to set up.",
    rating: 4,
    publish: true,
    consent: true,
  });
});

it("refuses staff before calling the backend", async () => {
  session.mockResolvedValue({
    role: "staff",
    userId: "staff",
    currentStore: { id: "real-store" },
  });
  expect(await saveTestimonial({}, form())).toMatchObject({ ok: false });
  expect(request).not.toHaveBeenCalled();
});

it("requires explicit consent to publish and validates the rating", async () => {
  const data = form();
  data.delete("consent");
  expect(await saveTestimonial({}, data)).toMatchObject({ ok: false });
  data.set("consent", "on");
  data.set("rating", "0");
  expect(await saveTestimonial({}, data)).toMatchObject({ ok: false });
  expect(request).not.toHaveBeenCalled();
});

it("saves privately without consent and preserves a retryable error", async () => {
  const data = form();
  data.set("intent", "draft");
  data.delete("consent");
  request.mockRejectedValue(new Error("offline"));
  expect(await saveTestimonial({}, data)).toMatchObject({
    ok: false,
    message: expect.stringContaining("try again"),
  });
  expect(JSON.parse(request.mock.calls[0][1].body)).toMatchObject({
    publish: false,
    consent: false,
  });
});
