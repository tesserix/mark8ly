import { notFound } from "next/navigation";

import { AdminShell } from "@/components/shell/AdminShell";
import { CustomerActionsBar } from "@/components/customers/CustomerActionsBar";
import { CustomerAddressesCard } from "@/components/customers/CustomerAddressesCard";
import { CustomerDetailHeader } from "@/components/customers/CustomerDetailHeader";
import { CustomerNotesEditor } from "@/components/customers/CustomerNotesEditor";
import { CustomerOverviewCard } from "@/components/customers/CustomerOverviewCard";
import { CustomerTagsEditor } from "@/components/customers/CustomerTagsEditor";
import { getCustomer } from "@/lib/api/marketplace-api";
import { getServerSessionContext } from "@/lib/auth/serverSession";

import {
  blockCustomerAction,
  unblockCustomerAction,
  updateNotesAction,
  updateTagsAction,
} from "./actions";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function CustomerDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) {
    notFound();
  }

  const customer = await getCustomer(currentStore.id, id, { userId, tenantId });
  if (!customer) {
    notFound();
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main
        className="mx-auto flex w-full max-w-5xl flex-col gap-10"
        aria-labelledby="customer-heading"
      >
        <CustomerDetailHeader customer={customer} />

        <div className="grid grid-cols-1 gap-10 lg:grid-cols-[2fr_1fr]">
          <div className="flex flex-col gap-10">
            <CustomerOverviewCard customer={customer} />
            <CustomerAddressesCard addresses={customer.addresses} />
            <CustomerTagsEditor
              customerId={customer.id}
              initialTags={customer.tags}
              updateAction={updateTagsAction}
            />
            <CustomerNotesEditor
              customerId={customer.id}
              initialNotes={customer.notes ?? ""}
              updateAction={updateNotesAction}
            />
          </div>
          <div className="flex flex-col gap-10 lg:sticky lg:top-8 lg:self-start">
            <CustomerActionsBar
              customerId={customer.id}
              status={customer.status}
              blockAction={blockCustomerAction}
              unblockAction={unblockCustomerAction}
            />
          </div>
        </div>
      </main>
    </AdminShell>
  );
}
