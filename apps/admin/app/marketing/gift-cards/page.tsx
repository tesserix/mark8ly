import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listGiftCards, type ListGiftCardsQuery } from "@/lib/api/marketplace-api";
import { GiftCardsList } from "@/components/marketing/gift-cards/GiftCardsList";
import { GiftCardsListEmpty } from "@/components/marketing/gift-cards/GiftCardsListEmpty";
import Link from "next/link";

interface GiftCardsPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function GiftCardsPage({ searchParams }: GiftCardsPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;
  const params = await searchParams;

  const canIssue = role === "owner" || role === "admin";

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <h1 className="font-serif text-2xl text-ink-900">Gift Cards</h1>
          <GiftCardsListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const query: ListGiftCardsQuery = {
    status: (params.status as ListGiftCardsQuery["status"]) ?? undefined,
    page: params.page ? Number(params.page) : 1,
    pageSize: params.page_size ? Number(params.page_size) : 20,
  };

  const response = await listGiftCards(currentStore.id, query, { userId, tenantId });
  const giftCards = response?.data ?? [];

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <div className="flex items-center justify-between">
          <h1 className="font-serif text-2xl text-ink-900">Gift Cards</h1>
          {canIssue && (
            <Link
              href="/marketing/gift-cards/new"
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition-colors hover:bg-moss-700"
            >
              Issue Gift Card
            </Link>
          )}
        </div>

        {giftCards.length === 0 ? (
          <GiftCardsListEmpty variant="no-gift-cards" canIssue={canIssue} />
        ) : (
          <GiftCardsList
            giftCards={giftCards}
            meta={response?.meta}
            currentStatus={query.status}
          />
        )}
      </main>
    </AdminShell>
  );
}
