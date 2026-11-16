import { z } from "zod";

const publicReview = z.object({
  store_id: z.string(),
  store_name: z.string(),
  author_name: z.string(),
  quote: z.string(),
  rating: z.number().int().min(1).max(5),
});
const responseSchema = z.object({ data: z.array(publicReview).max(12) });
export type PublicTestimonial = z.infer<typeof publicReview>;

export async function loadTestimonials(
  base: string,
): Promise<PublicTestimonial[]> {
  const res = await fetch(`${base}/api/v1/testimonials`, {
    cache: "no-store",
    signal: AbortSignal.timeout(4000),
  });
  if (!res.ok) throw new Error("Testimonials unavailable");
  return responseSchema.parse(await res.json()).data;
}
