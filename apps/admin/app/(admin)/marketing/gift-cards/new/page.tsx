import { getServerSessionContext } from "@/lib/auth/serverSession";
import { IssueGiftCardForm } from "@/components/marketing/gift-cards/IssueGiftCardForm";

export default async function IssueGiftCardPage() {
  const session = await getServerSessionContext();

  if (!session.currentStore) {
    return (
      <main className="flex flex-col gap-8">
        <h1 className="font-serif text-2xl text-ink-900">No store selected</h1>
        <p className="text-sm text-ink-500">
          Set up a store before issuing gift cards.
        </p>
      </main>
    );
  }

  return (
    <main className="flex flex-col gap-8">
      <h1 className="font-serif text-2xl text-ink-900">Issue Gift Card</h1>
      <IssueGiftCardForm currency={session.currentStore.currency_code} />
    </main>
  );
}
