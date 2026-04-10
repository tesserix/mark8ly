import Link from "next/link";

interface CouponsListEmptyProps {
  variant: "no-coupons" | "no-store";
}

export function CouponsListEmpty({ variant }: CouponsListEmptyProps) {
  if (variant === "no-store") {
    return (
      <div className="flex flex-col items-start gap-2 border-t border-ink-200 px-0 py-12">
        <p className="text-sm text-ink-600">
          Create a store first to start managing coupons.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-start gap-4 border-t border-ink-200 px-0 py-12">
      <h2 className="font-serif text-lg font-semibold text-ink-900">
        No coupons yet
      </h2>
      <p className="text-sm text-ink-600">
        Create your first coupon to offer discounts to your customers.
      </p>
      <Link
        href="/marketing/coupons/new"
        aria-label="Create your first coupon"
        className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
      >
        Create coupon
      </Link>
    </div>
  );
}
