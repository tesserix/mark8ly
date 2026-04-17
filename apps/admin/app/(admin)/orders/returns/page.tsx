// Admin RMA inbox — every return/replace request for the current store,
// with approve / reject / pickup-details actions. The server component
// shell loads the initial list; interactions happen in the client
// component (ReturnsInbox) via the same-origin proxy under
// /api/admin/stores/:storeId/returns/*.

import { AdminPage } from "@/components/layout";
import { ReturnsInbox } from "@/components/orders/ReturnsInbox";
import { listReturns } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

const DESCRIPTION =
  "Customer return and replacement requests. Approve to trigger pickup / reship, or reject with a reason.";

export default async function ReturnsPage() {
  const session = await getServerSessionContext();
  const { currentStore, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminPage eyebrow="Operations" title="Returns & replacements" description={DESCRIPTION}>
        <p className="text-sm text-foreground-tertiary">
          Select a store to view return requests.
        </p>
      </AdminPage>
    );
  }

  const initialReturns =
    (await listReturns(currentStore.id, { userId, tenantId })) ?? [];

  return (
    <AdminPage
      eyebrow="Operations"
      title="Returns & replacements"
      description={DESCRIPTION}
    >
      <ReturnsInbox
        storeId={currentStore.id}
        initialReturns={initialReturns}
      />
    </AdminPage>
  );
}
