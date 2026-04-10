import Link from "next/link";

interface GiftCardsListEmptyProps {
  variant: "no-store" | "no-gift-cards";
  canIssue?: boolean;
}

export function GiftCardsListEmpty({ variant, canIssue }: GiftCardsListEmptyProps) {
  if (variant === "no-store") {
    return (
      <div className="flex flex-col items-center gap-3 py-16 text-center">
        <p className="text-ink-900/50">Select a store to manage gift cards.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center gap-4 py-16 text-center">
      <p className="text-lg text-ink-900/70">No gift cards issued yet</p>
      <p className="max-w-md text-sm text-ink-900/50">
        Gift cards let your customers give store credit as gifts. Issue your
        first gift card to get started.
      </p>
      {canIssue && (
        <Link
          href="/marketing/gift-cards/new"
          className="mt-2 inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700"
        >
          Issue Gift Card
        </Link>
      )}
    </div>
  );
}
