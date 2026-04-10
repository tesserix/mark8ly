import Link from "next/link";

interface CouponsListHeaderProps {
  canCreate: boolean;
}

export function CouponsListHeader({ canCreate }: CouponsListHeaderProps) {
  return (
    <div className="flex items-center justify-between">
      <div>
        <h1 className="font-serif text-2xl font-semibold text-ink-900">
          Coupons
        </h1>
        <p className="mt-1 text-sm text-ink-500">
          Create and manage discount coupons for your store.
        </p>
      </div>
      {canCreate && (
        <Link
          href="/marketing/coupons/new"
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
        >
          Create coupon
        </Link>
      )}
    </div>
  );
}
