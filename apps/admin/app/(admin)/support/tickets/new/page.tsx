import { getServerSessionContext } from "@/lib/auth/serverSession";
import { TicketCreateForm } from "@/components/support/TicketCreateForm";

export default async function NewTicketPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
  } = await getServerSessionContext();

  return (
          <div className="space-y-8">
        <header className="space-y-3">
          <p className="eyebrow">Support</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            New ticket
          </h1>
          <p className="max-w-xl text-base leading-7 text-foreground-secondary">
            Describe your issue and our support team will get back to you as
            soon as possible.
          </p>
        </header>

        <TicketCreateForm />
      </div>
  );
}
