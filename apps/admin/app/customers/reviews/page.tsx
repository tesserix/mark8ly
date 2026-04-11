import { AdminShell } from "@/components/shell/AdminShell";
import { ReviewsList } from "@/components/customers/reviews/ReviewsList";
import { ReviewsListEmpty } from "@/components/customers/reviews/ReviewsListEmpty";
import { ReviewsListHeader } from "@/components/customers/reviews/ReviewsListHeader";
import { CustomersListPagination } from "@/components/customers/CustomersListPagination";
import {
  getProduct,
  listReviews,
  type AdminReview,
  type ListReviewsQuery,
  type ReviewStatus,
  type SessionHeaders,
} from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

interface ReviewsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

const VALID_STATUS: readonly ReviewStatus[] = [
  "pending",
  "approved",
  "rejected",
];

export default async function ReviewsPage({ searchParams }: ReviewsPageProps) {
  const session = await getServerSessionContext();
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = session;
  const params = await searchParams;
  const query = parseSearchParams(params);

  if (!currentStore) {
    return (
      <AdminShell
        tenantName={tenantName}
        userEmail={email}
        role={role}
        memberships={memberships}
        currentTenantId={tenantId}
      >
        <section
          className="flex flex-col gap-8"
          aria-labelledby="reviews-heading"
        >
          <ReviewsListHeader activeStatus={query.status} />
          <ReviewsListEmpty variant="no-reviews" />
        </section>
      </AdminShell>
    );
  }

  const response = await listReviews(currentStore.id, query, {
    userId,
    tenantId,
  });

  const reviews = response?.data ?? [];
  const meta = response?.meta ?? {
    page: 1,
    page_size: query.pageSize ?? 20,
    total: 0,
    total_pages: 0,
  };
  const hasActiveFilters = !!query.status;
  const isEmpty = reviews.length === 0;

  const productTitles = await resolveProductTitles(
    currentStore.id,
    reviews,
    { userId, tenantId },
  );

  const buildHref = (page: number) => {
    const p = new URLSearchParams();
    if (query.status) p.set("status", query.status);
    if (query.pageSize) p.set("page_size", String(query.pageSize));
    if (page > 1) p.set("page", String(page));
    const qs = p.toString();
    return qs ? `/customers/reviews?${qs}` : "/customers/reviews";
  };

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <section
        className="flex flex-col gap-8"
        aria-labelledby="reviews-heading"
      >
        <ReviewsListHeader activeStatus={query.status} />

        {isEmpty ? (
          <ReviewsListEmpty
            variant={hasActiveFilters ? "no-matches" : "no-reviews"}
            clearFiltersHref={hasActiveFilters ? "/customers/reviews" : undefined}
          />
        ) : (
          <>
            <ReviewsList
              reviews={reviews}
              productTitles={productTitles}
            />
            <CustomersListPagination
              currentPage={meta.page}
              totalPages={meta.total_pages}
              buildHref={buildHref}
            />
          </>
        )}
      </section>
    </AdminShell>
  );
}

function parseSearchParams(
  raw: Record<string, string | string[] | undefined>,
): ListReviewsQuery {
  const statusRaw = typeof raw.status === "string" ? raw.status : undefined;
  const status = VALID_STATUS.includes(statusRaw as ReviewStatus)
    ? (statusRaw as ReviewStatus)
    : undefined;

  const pageRaw =
    typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSizeRaw =
    typeof raw.page_size === "string"
      ? Number.parseInt(raw.page_size, 10)
      : undefined;

  return {
    status,
    page: pageRaw && pageRaw > 0 ? pageRaw : undefined,
    pageSize:
      pageSizeRaw && pageSizeRaw > 0 && pageSizeRaw <= 100
        ? pageSizeRaw
        : undefined,
  };
}

/**
 * Resolve unique product titles referenced by the current page of reviews.
 * Runs a parallel getProduct fetch per unique id so the UI can show a
 * readable title instead of a raw UUID. Failures fall back to undefined
 * and the row will render the short id as a graceful degradation.
 */
async function resolveProductTitles(
  storeId: string,
  reviews: AdminReview[],
  session: SessionHeaders,
): Promise<Record<string, string>> {
  const uniqueIds = Array.from(new Set(reviews.map((r) => r.product_id)));
  if (uniqueIds.length === 0) return {};

  const entries = await Promise.all(
    uniqueIds.map(async (id): Promise<[string, string] | null> => {
      try {
        const product = await getProduct(storeId, id, session);
        return product ? [id, product.title] : null;
      } catch {
        return null;
      }
    }),
  );

  const titles: Record<string, string> = {};
  for (const entry of entries) {
    if (entry) {
      titles[entry[0]] = entry[1];
    }
  }
  return titles;
}
