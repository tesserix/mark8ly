import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getTicket } from "@/lib/api/marketplace-api";
import { Breadcrumbs } from "@/components/layout";
import { TicketDetail } from "@/components/support/TicketDetail";

interface TicketDetailPageProps {
  params: Promise<{ id: string }>;
}

export default async function TicketDetailPage({
  params,
}: TicketDetailPageProps) {
  const { id } = await params;
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const ticket =
    currentStore
      ? await getTicket(currentStore.id, id, { userId, tenantId })
      : null;

  return (
    <div className="flex flex-col gap-8">
      <Breadcrumbs
        items={[
          { label: "Support", href: "/support" },
          { label: "Tickets", href: "/support/tickets" },
          { label: ticket ? `#${ticket.id.slice(0, 8)}` : "Not found" },
        ]}
      />

      {!ticket ? (
        <div className="py-12 text-center" role="alert">
          <h2 className="font-serif text-2xl font-medium text-foreground">
            Ticket not found
          </h2>
          <p className="mt-2 text-sm text-foreground-secondary">
            This ticket may have been deleted or you do not have access to it.
          </p>
        </div>
      ) : (
        <TicketDetail ticket={ticket} />
      )}
    </div>
  );
}
