import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/components/layout";
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

  const displayName = [customer.first_name, customer.last_name]
    .filter(Boolean)
    .join(" ") || customer.email;

  return (
    <main className="flex flex-col gap-10" aria-labelledby="customer-heading">
      <Breadcrumbs
        items={[
          { label: "Customers", href: "/customers" },
          { label: displayName },
        ]}
      />

      <CustomerDetailHeader customer={customer} />

      <div className="grid grid-cols-1 gap-10 lg:grid-cols-[minmax(0,1fr)_280px]">
        <div className="flex min-w-0 flex-col gap-10">
          <CustomerOverviewCard
            customer={customer}
            currency={currentStore.currency_code}
          />
          <CustomerAddressesCard addresses={customer.addresses} />
        </div>

        <aside className="flex flex-col gap-8 lg:sticky lg:top-8 lg:self-start">
          <CustomerTagsEditor
            customerId={customer.id}
            initialTags={customer.tags}
            updateAction={updateTagsAction}
          />
          <div className="border-t border-border-subtle pt-8">
            <CustomerNotesEditor
              customerId={customer.id}
              initialNotes={customer.notes ?? ""}
              updateAction={updateNotesAction}
            />
          </div>
          <div className="border-t border-border-subtle pt-8">
            <CustomerActionsBar
              customerId={customer.id}
              status={customer.status}
              blockAction={blockCustomerAction}
              unblockAction={unblockCustomerAction}
            />
          </div>
        </aside>
      </div>
    </main>
  );
}
