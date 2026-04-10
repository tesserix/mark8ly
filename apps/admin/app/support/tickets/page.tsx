import Link from "next/link";
import { Plus } from "lucide-react";

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  listTickets,
  type TicketStatus,
} from "@/lib/api/marketplace-api";
import { TicketsList } from "@/components/support/TicketsList";

interface TicketsPageProps {
  searchParams: Promise<{ status?: string; search?: string; page?: string }>;
}

export default async function TicketsPage({ searchParams }: TicketsPageProps) {
  const params = await searchParams;
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    userId,
    currentStore,
  } = await getServerSessionContext();

  const status = (params.status ?? "open") as TicketStatus;
  const search = params.search ?? "";
  const page = Number(params.page) || 1;

  const ticketsData = currentStore
    ? await listTickets(
        currentStore.id,
        { status, search: search || undefined, page },
        { userId, tenantId },
      )
    : null;

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-5xl space-y-8">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div className="space-y-3">
            <p className="eyebrow">Support</p>
            <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
              Tickets
            </h1>
            <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
              Manage your support requests and communicate with the platform
              team.
            </p>
          </div>
          <Link
            href="/support/tickets/new"
            className="inline-flex h-11 min-h-[44px] shrink-0 items-center gap-2 rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground hover:bg-primary-hover"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            New ticket
          </Link>
        </header>

        {!currentStore ? (
          <p className="text-sm text-danger" role="alert">
            No store found. Please create a store before submitting tickets.
          </p>
        ) : ticketsData ? (
          <TicketsList
            tickets={ticketsData.data}
            counts={ticketsData.counts}
            activeStatus={status}
          />
        ) : (
          <p className="text-sm text-foreground-secondary" role="alert">
            Unable to load tickets. Please try again later.
          </p>
        )}
      </div>
    </AdminShell>
  );
}
