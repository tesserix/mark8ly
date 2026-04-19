// Same-origin proxy for Stripe billing webhooks. Stripe posts to
// https://admin.mark8ly.com/webhooks/stripe-billing — this route forwards
// the raw body + Stripe-Signature header verbatim to marketplace-api-admin
// so the Go handler can verify the HMAC signature against the untouched
// bytes. Any reparsing or framework middleware handling of the body would
// break signature verification.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const HOP_BY_HOP = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
  "content-encoding",
  "content-length",
]);

export async function POST(request: Request): Promise<Response> {
  const body = await request.arrayBuffer();

  const upstreamHeaders: Record<string, string> = {
    Accept: "application/json",
  };
  const sig = request.headers.get("stripe-signature");
  if (sig) upstreamHeaders["Stripe-Signature"] = sig;
  const contentType = request.headers.get("content-type");
  if (contentType) upstreamHeaders["Content-Type"] = contentType;

  let upstream: Response;
  try {
    upstream = await fetch(
      `${MARKETPLACE_API_URL}/webhooks/stripe-billing`,
      {
        method: "POST",
        headers: upstreamHeaders,
        body,
        redirect: "manual",
      },
    );
  } catch (err) {
    const message =
      err instanceof Error ? err.message : "upstream fetch failed";
    return new Response(
      JSON.stringify({ error: "bad_gateway", message }),
      { status: 502, headers: { "Content-Type": "application/json" } },
    );
  }

  const responseHeaders = new Headers();
  upstream.headers.forEach((value, key) => {
    if (!HOP_BY_HOP.has(key.toLowerCase())) {
      responseHeaders.set(key, value);
    }
  });

  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  });
}
