import { marketplaceStoreUrl, proxyJson, requireStoreSlug } from "../../../../checkout/_proxy";
import { readCartToken } from "../../../_cart";

export const dynamic = "force-dynamic";

/**
 * Records the buyer's crop rectangle and returns somewhere to put the
 * preview (#966).
 *
 * The response carries a signed GET for the PRISTINE original — the
 * thing the crop is computed against — and a signed PUT for the derived
 * preview. The original is never overwritten, so a re-crop starts from
 * the full-resolution image rather than from the last preview, and the
 * buyer can crop repeatedly without compounding loss.
 *
 * The cart token comes from the httpOnly cookie, never the body: it is
 * the only thing proving this upload is the caller's.
 */
export async function PATCH(
  req: Request,
  { params }: { params: Promise<{ uploadId: string }> },
): Promise<Response> {
  const parsed = requireStoreSlug(req);
  if (parsed instanceof Response) return parsed;

  const cartToken = await readCartToken();
  if (!cartToken) {
    return Response.json({ error: "not_found", message: "upload not found" }, { status: 404 });
  }

  let crop: unknown;
  try {
    const body = (await req.json()) as { crop?: unknown };
    crop = body?.crop;
  } catch {
    return Response.json(
      { error: "invalid_request", message: "crop request could not be parsed" },
      { status: 400 },
    );
  }

  const { uploadId } = await params;
  const url = new URL(
    `${marketplaceStoreUrl(parsed.slug)}/personalisation/uploads/${encodeURIComponent(uploadId)}/crop`,
  );
  return proxyJson(url.toString(), {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ crop, cart_token: cartToken }),
  });
}
