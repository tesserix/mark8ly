"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";

export type TestimonialState = {
  ok?: boolean;
  message?: string;
  published?: boolean;
};

const reviewSchema = z.object({
  quote: z
    .string()
    .trim()
    .min(1)
    .refine((v) => Array.from(v).length <= 1000),
  rating: z.number().int().min(1).max(5),
  // Blank or unticked means the merchant chose not to show their name.
  display_name: z
    .string()
    .trim()
    .refine((v) => Array.from(v).length <= 80)
    .transform((v) => v || null),
});

export async function saveTestimonial(
  _previous: TestimonialState,
  data: FormData,
): Promise<TestimonialState> {
  const { role, currentStore, userId } = await getServerSessionContext();
  if ((role !== "owner" && role !== "admin") || !currentStore || !userId) {
    return {
      ok: false,
      message: "Only a store owner or admin can submit a testimonial.",
    };
  }
  const parsed = reviewSchema.safeParse({
    quote: data.get("quote"),
    rating: Number(data.get("rating")),
    display_name:
      data.get("show_name") === "on" ? String(data.get("display_name") ?? "") : "",
  });
  if (!parsed.success)
    return {
      ok: false,
      message:
        "Enter your review (up to 1,000 characters), a 1–5 star rating, and a name of up to 80 characters if you show one.",
    };
  const intent = data.get("intent");
  if (intent !== "publish" && intent !== "draft")
    return {
      ok: false,
      message: "Choose whether to publish or save privately.",
    };
  const publish = intent === "publish";
  const consent = data.get("consent") === "on";
  if (publish && !consent)
    return {
      ok: false,
      message: "Please agree to share your review before publishing.",
    };
  try {
    const res = await platformInternalFetch(
      `/internal/stores/${encodeURIComponent(currentStore.id)}/testimonial`,
      {
        method: "PUT",
        cache: "no-store",
        signal: AbortSignal.timeout(5000),
        headers: { "Content-Type": "application/json", "X-User-ID": userId },
        body: JSON.stringify({ ...parsed.data, publish, consent }),
      },
    );
    if (!res.ok)
      return {
        ok: false,
        message: "Could not save your testimonial. Please try again.",
      };
    revalidatePath("/settings/testimonial");
    return {
      ok: true,
      published: publish,
      message: publish
        ? "Published. Your review can now appear on mark8ly.com."
        : "Saved privately. Your review will not appear on mark8ly.com.",
    };
  } catch {
    return {
      ok: false,
      message: "Could not save your testimonial. Please try again.",
    };
  }
}
