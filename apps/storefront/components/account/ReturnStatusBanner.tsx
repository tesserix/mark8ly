// Server component rendered inside /account/orders/[id].
// Fetches the order's return requests server-side (so the banner is
// already filled in on first paint) and renders the appropriate state.
//
// States:
//   requested → "Awaiting review" banner with case number
//   approved  → "Approved" banner + pickup_details if populated
//   rejected  → "Rejected" banner with reason
//   received  → "We've received your package"
//   refunded  → "Refund processed" (soft — totals show the refund line too)

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";

interface Props {
  orderId: string;
}

interface StorefrontReturn {
  id: string;
  return_number: string;
  type: "return" | "replace";
  status: "requested" | "approved" | "received" | "refunded" | "rejected";
  reason?: string;
  notes?: string;
  pickup_details?: string;
  reject_reason?: string;
  currency_code: string;
  requested_at: string;
  approved_at?: string;
  rejected_at?: string;
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

async function fetchReturns(
  slug: string,
  orderId: string,
  sessionCookie: string,
): Promise<StorefrontReturn[]> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(slug)}/orders/${encodeURIComponent(orderId)}/returns`;
  try {
    const res = await fetch(url, {
      method: "GET",
      headers: {
        Accept: "application/json",
        "X-Storefront-Key": STOREFRONT_KEY,
        Cookie: sessionCookie ? `mp_customer_session=${sessionCookie}` : "",
      },
      cache: "no-store",
    });
    if (!res.ok) return [];
    const body = (await res.json()) as { returns?: StorefrontReturn[] };
    return body.returns ?? [];
  } catch {
    return [];
  }
}

export async function ReturnStatusBanner({ orderId }: Props) {
  const h = await headers();
  const slug = await resolveStoreSlug(h.get("host"));
  if (!slug) return null;
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value ?? "";
  if (!session) return null;

  const returns = await fetchReturns(slug, orderId, session);
  // Show the most recent active (non-final) return first; if everything
  // is terminal we still show the latest so the customer has record of
  // what happened.
  if (returns.length === 0) return null;
  const active =
    returns.find((r) => r.status !== "refunded" && r.status !== "rejected") ??
    returns[0];
  if (!active) return null;
  return <Banner r={active} />;
}

function Banner({ r }: { r: StorefrontReturn }) {
  const typeLabel = r.type === "replace" ? "Replacement" : "Return";
  switch (r.status) {
    case "requested":
      return (
        <ShellBanner tone="neutral">
          <TitleLine>
            {typeLabel} request submitted · <Mono>{r.return_number}</Mono>
          </TitleLine>
          <BodyLine>
            We&apos;ve received your {r.type === "replace" ? "replacement" : "return"} request
            and the store team will review it shortly. You&apos;ll see the next step
            here as soon as they approve it.
          </BodyLine>
          {r.reason ? <MetaLine label="Reason">{r.reason}</MetaLine> : null}
        </ShellBanner>
      );
    case "approved":
      return (
        <ShellBanner tone="success">
          <TitleLine>
            {typeLabel} approved · <Mono>{r.return_number}</Mono>
          </TitleLine>
          {r.pickup_details ? (
            <BodyLine preserveWhitespace>{r.pickup_details}</BodyLine>
          ) : (
            <BodyLine>
              Approved — our team will reach out with pickup or shipping
              details shortly.
            </BodyLine>
          )}
        </ShellBanner>
      );
    case "received":
      return (
        <ShellBanner tone="success">
          <TitleLine>
            Package received · <Mono>{r.return_number}</Mono>
          </TitleLine>
          <BodyLine>
            {r.type === "replace"
              ? "Your replacement is being prepared for shipment."
              : "We&apos;ve received your return and your refund is being processed."}
          </BodyLine>
        </ShellBanner>
      );
    case "refunded":
      return (
        <ShellBanner tone="success">
          <TitleLine>
            Refund issued · <Mono>{r.return_number}</Mono>
          </TitleLine>
          <BodyLine>
            The amount appears as a refund line in your order totals below.
          </BodyLine>
        </ShellBanner>
      );
    case "rejected":
      return (
        <ShellBanner tone="warn">
          <TitleLine>
            {typeLabel} request declined · <Mono>{r.return_number}</Mono>
          </TitleLine>
          {r.reject_reason ? (
            <MetaLine label="Reason">{r.reject_reason}</MetaLine>
          ) : (
            <BodyLine>
              Our team couldn&apos;t approve this request. Please reply to your
              order confirmation email if you have questions.
            </BodyLine>
          )}
        </ShellBanner>
      );
    default:
      return null;
  }
}

function ShellBanner({
  tone,
  children,
}: {
  tone: "neutral" | "success" | "warn";
  children: React.ReactNode;
}) {
  const bg =
    tone === "success"
      ? "border-emerald-600/20 bg-emerald-50/70"
      : tone === "warn"
        ? "border-amber-600/30 bg-amber-50/70"
        : "border-[color:var(--storefront-text,var(--ink-900))]/15 bg-[color:var(--storefront-text,var(--ink-900))]/[0.02]";
  return (
    <section
      className={`rounded-md border ${bg} px-4 py-3 text-sm text-[color:var(--storefront-text,var(--ink-900))]`}
      aria-live="polite"
    >
      <div className="space-y-1">{children}</div>
    </section>
  );
}

function TitleLine({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-[13px] font-semibold">
      {children}
    </div>
  );
}

function BodyLine({
  children,
  preserveWhitespace = false,
}: {
  children: React.ReactNode;
  preserveWhitespace?: boolean;
}) {
  return (
    <p
      className={`text-[13px] leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]/80 ${
        preserveWhitespace ? "whitespace-pre-wrap" : ""
      }`}
    >
      {children}
    </p>
  );
}

function MetaLine({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <p className="text-[12px] text-[color:var(--storefront-text,var(--ink-900))]/65">
      <span className="font-medium opacity-80">{label}:</span> {children}
    </p>
  );
}

function Mono({ children }: { children: React.ReactNode }) {
  return (
    <span className="font-mono tracking-tight text-[12px] opacity-80">
      {children}
    </span>
  );
}
