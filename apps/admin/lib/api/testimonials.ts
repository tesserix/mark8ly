import { platformInternalFetch } from "@/lib/api/server/platformInternal";

export type MerchantTestimonial = {
  position: "Store owner" | "Store admin";
  quote: string;
  rating: number;
  published: boolean;
};

export async function getTestimonial(
  storeId: string,
  userId: string,
): Promise<MerchantTestimonial | null> {
  const res = await platformInternalFetch(
    `/internal/stores/${encodeURIComponent(storeId)}/testimonial`,
    {
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
      headers: { "X-User-ID": userId },
    },
  );
  if (!res.ok) throw new Error("Could not load testimonial");
  const body = (await res.json()) as { data: MerchantTestimonial | null };
  return body.data;
}
