import Link from "next/link";
import { ChevronLeft } from "lucide-react";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getTicket } from "@/lib/api/marketplace-api";
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
          <div className="space-y-6">
        <Link
          href="/support/tickets"
          className="inline-flex min-h-[44px] items-center gap-1 text-sm text-foreground-secondary hover:text-foreground"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          Back to tickets
        </Link>

        {!ticket ? (
          <div className="py-12 text-center" role="alert">
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
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
