import { config } from "@/lib/config";
import { loadTestimonials } from "@/lib/testimonials";

export const dynamic = "force-dynamic";

export async function GET() {
  const headers = { "Cache-Control": "no-store" };
  try {
    return Response.json(
      { data: await loadTestimonials(config.platformApiUrl) },
      { headers },
    );
  } catch {
    return Response.json({ data: [] }, { status: 503, headers });
  }
}
