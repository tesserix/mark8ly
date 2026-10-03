"use client";

// components/orders/OrderArtworkPanel.tsx
//
// What the buyer asked for, and how the merchant gets hold of it (#968).
//
// With image-to-3D deferred this panel IS the custom-products feature on
// the merchant's side: the photograph and the text are the whole of the
// brief. So the design priority is that nothing here can quietly fail —
// a merchant who thinks they have the artwork and does not will ship the
// wrong thing to a paying customer.
//
// Two decisions worth keeping:
//
//   * Links are minted on click, never on render. Each one is a signed
//     URL that expires in minutes and writes an audit row; fetching them
//     up front would mean a wall of dead links and an audit trail that
//     records page views rather than access.
//   * The download is the PRISTINE ORIGINAL. The buyer's crop is shown
//     as numbers beside it, not applied — the preview they saw is a
//     re-encoded thumbnail, and printing from it ships a blurry product.

import { useCallback, useState } from "react";

import { useToast } from "@/components/feedback/Toaster";
import type {
  AdminArtworkLink,
  AdminOrderItem,
  AdminOrderPersonalisation,
} from "@/lib/api/marketplace-api";

interface OrderArtworkPanelProps {
  storeId: string;
  orderId: string;
  items: AdminOrderItem[];
}

/** A line that actually carries buyer input, paired with its answers. */
interface AnsweredLine {
  item: AdminOrderItem;
  answers: AdminOrderPersonalisation[];
}

function answeredLines(items: AdminOrderItem[]): AnsweredLine[] {
  return items
    .filter((it) => (it.personalisation?.length ?? 0) > 0)
    .map((it) => ({
      item: it,
      answers: [...(it.personalisation ?? [])].sort(
        (a, b) => a.position - b.position,
      ),
    }));
}

function formatSize(bytes?: number): string | null {
  if (bytes === undefined || bytes <= 0) return null;
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function OrderArtworkPanel({
  storeId,
  orderId,
  items,
}: OrderArtworkPanelProps) {
  const lines = answeredLines(items);
  const { toast } = useToast();
  // Keyed by personalisation id so two downloads in flight cannot
  // overwrite each other's spinner.
  const [pending, setPending] = useState<Record<string, boolean>>({});
  const [busyAll, setBusyAll] = useState(false);

  const openLink = useCallback((link: AdminArtworkLink) => {
    // A new tab rather than a navigation: the merchant is mid-task on
    // this page and should come back to it, and GCS answers a signed GET
    // with a download, so nothing visible is lost.
    window.open(link.url, "_blank", "noopener,noreferrer");
  }, []);

  const downloadOne = useCallback(
    async (p: AdminOrderPersonalisation) => {
      setPending((prev) => ({ ...prev, [p.id]: true }));
      try {
        const res = await fetch(
          `/api/admin/stores/${storeId}/orders/${orderId}` +
            `/personalisations/${p.id}/download`,
          { cache: "no-store" },
        );
        if (!res.ok) {
          toast.error(
            "Could not fetch the artwork",
            res.status === 501
              ? "Artwork storage is not configured for this environment."
              : "The file could not be reached. Try again, or contact support before shipping this order.",
          );
          return;
        }
        openLink((await res.json()) as AdminArtworkLink);
      } catch {
        toast.error(
          "Could not fetch the artwork",
          "Check your connection and try again.",
        );
      } finally {
        setPending((prev) => ({ ...prev, [p.id]: false }));
      }
    },
    [openLink, orderId, toast, storeId],
  );

  const downloadAll = useCallback(async () => {
    setBusyAll(true);
    try {
      const res = await fetch(
        `/api/admin/stores/${storeId}/orders/${orderId}/personalisations/download`,
        { cache: "no-store" },
      );
      if (!res.ok) {
        toast.error(
          "Could not fetch the artwork",
          "No files were opened. Try the individual downloads below.",
        );
        return;
      }
      const body = (await res.json()) as { artwork: AdminArtworkLink[] };
      // Sequentially, because opening six tabs in one tick is how a
      // popup blocker swallows five of them silently.
      for (const link of body.artwork) {
        openLink(link);
      }
      if (body.artwork.length === 0) {
        toast.info(
          "Nothing to download",
          "This order has no uploaded files.",
        );
      }
    } catch {
        toast.error(
          "Could not fetch the artwork",
          "Check your connection and try again.",
        );
    } finally {
      setBusyAll(false);
    }
  }, [openLink, orderId, toast, storeId]);

  if (lines.length === 0) return null;

  const artworkCount = lines.reduce(
    (n, l) => n + l.answers.filter((a) => a.has_artwork).length,
    0,
  );

  return (
    <section
      aria-labelledby="order-artwork-heading"
      className="flex flex-col gap-4"
    >
      <div className="flex items-end justify-between gap-6">
        <div className="flex flex-col gap-1">
          <h2
            id="order-artwork-heading"
            className="font-serif text-2xl font-medium text-foreground"
          >
            Personalisation
          </h2>
          <p className="text-xs text-foreground-tertiary">
            What the customer asked for. Downloads are the original files at
            full resolution — print from these, not from the preview.
          </p>
        </div>
        {artworkCount > 1 && (
          <button
            type="button"
            onClick={downloadAll}
            disabled={busyAll}
            className="shrink-0 rounded-sm border border-[color:var(--ink-900)]/20 px-3 py-2 text-xs font-medium text-foreground transition hover:bg-[color:var(--ink-900)]/5 disabled:opacity-50"
          >
            {busyAll ? "Opening…" : `Download all (${artworkCount})`}
          </button>
        )}
      </div>

      <ul role="list" className="flex flex-col gap-4">
        {lines.map(({ item, answers }) => (
          <li
            key={item.id}
            className="flex flex-col gap-3 border-b border-[color:var(--ink-900)]/10 pb-4 last:border-b-0"
          >
            <span className="text-base text-foreground">
              {item.title_snapshot}
              {item.quantity > 1 && (
                <span className="text-foreground-tertiary">
                  {" "}
                  × {item.quantity}
                </span>
              )}
            </span>
            <dl className="flex flex-col gap-3">
              {answers.map((a) => (
                <div
                  key={a.id}
                  className="grid gap-x-6 gap-y-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] sm:items-start"
                >
                  <dt className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
                    {a.field_label}
                  </dt>
                  <dd className="flex flex-col gap-1 text-sm text-foreground">
                    {a.has_artwork ? (
                      <>
                        <span className="break-all">
                          {a.original_filename ?? "Uploaded file"}
                        </span>
                        <span className="text-xs text-foreground-tertiary">
                          Ref {a.reference}
                          {a.content_type && ` · ${a.content_type}`}
                          {formatSize(a.size_bytes) &&
                            ` · ${formatSize(a.size_bytes)}`}
                        </span>
                      </>
                    ) : (
                      // Text, select and checkbox answers. Pre-wrap
                      // because a buyer's textarea is theirs to lay out,
                      // and an engraving that loses its line breaks is
                      // the wrong engraving.
                      <span className="whitespace-pre-wrap break-words">
                        {a.text_value ?? "—"}
                      </span>
                    )}
                  </dd>
                  <dd className="sm:justify-self-end">
                    {a.has_artwork && (
                      <button
                        type="button"
                        onClick={() => void downloadOne(a)}
                        disabled={pending[a.id]}
                        className="rounded-sm border border-[color:var(--ink-900)]/20 px-3 py-1.5 text-xs font-medium text-foreground transition hover:bg-[color:var(--ink-900)]/5 disabled:opacity-50"
                      >
                        {pending[a.id] ? "Opening…" : "Download original"}
                      </button>
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          </li>
        ))}
      </ul>
    </section>
  );
}
