"use client";

import { useCallback, useState } from "react";
import { Check, Copy } from "lucide-react";
import type { ActivePromotion } from "@/lib/api/marketplace-api";

interface PromotionBarProps {
  promotion?: ActivePromotion;
  announcement?: {
    text?: string;
    link?: string;
    bg?: string;
    active: boolean;
  };
}

/**
 * Displays a single promotion/announcement strip between the nav and
 * the hero. Campaign promotions take precedence over the static
 * announcement bar. When neither is active, renders nothing.
 */
export function PromotionBar({ promotion, announcement }: PromotionBarProps) {
  const showCampaign = !!promotion;
  const showAnnouncement = !!announcement?.active && !!announcement.text;

  if (!showCampaign && !showAnnouncement) return null;

  return (
    <>
      {showAnnouncement ? (
        <AnnouncementStrip
          text={announcement.text!}
          link={announcement.link}
          bg={announcement.bg}
        />
      ) : null}
      {showCampaign ? <CampaignStrip promotion={promotion} /> : null}
    </>
  );
}

function CampaignStrip({ promotion }: { promotion: ActivePromotion }) {
  const [copied, setCopied] = useState(false);

  const copyCode = useCallback(() => {
    if (!promotion.coupon_code) return;
    navigator.clipboard.writeText(promotion.coupon_code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [promotion.coupon_code]);

  return (
    <div
      className="bg-[color:var(--storefront-text)] text-[color:var(--storefront-background)]"
    >
      <div className="mx-auto flex max-w-7xl items-center justify-center gap-3 px-4 py-2.5 text-sm">
        <p className="text-center">{promotion.label}</p>

        {promotion.coupon_code && (
          <button
            type="button"
            onClick={copyCode}
            className="inline-flex items-center gap-1.5 rounded border border-[color:var(--storefront-background)]/20 bg-[color:var(--storefront-background)]/10 px-2.5 py-1 font-mono text-xs font-medium tracking-wider transition hover:bg-[color:var(--storefront-background)]/20"
          >
            {promotion.coupon_code}
            {copied ? (
              <Check className="size-3" aria-hidden="true" />
            ) : (
              <Copy className="size-3" aria-hidden="true" />
            )}
          </button>
        )}
      </div>
    </div>
  );
}

function AnnouncementStrip({
  text,
  link,
  bg,
}: {
  text: string;
  link?: string;
  bg?: string;
}) {
  const style = bg ? { backgroundColor: bg } : undefined;
  const content = (
    <p className="text-center text-sm">
      {text}
    </p>
  );

  return (
    <div
      className="bg-[color:var(--storefront-text)] text-[color:var(--storefront-background)] px-4 py-2.5"
      style={style}
    >
      <div className="mx-auto max-w-7xl">
        {link ? (
          <a
            href={link}
            className="block underline-offset-2 hover:underline"
          >
            {content}
          </a>
        ) : (
          content
        )}
      </div>
    </div>
  );
}
