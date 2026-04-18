import { getServerSessionContext } from "@/lib/auth/serverSession";
import { OttoAuditLog } from "@/components/support/OttoAuditLog";

// Store-wide Otto audit tail — shows the latest 100 significant
// events (case created, accepted, closed, reopened, feedback
// submitted, staff availability toggles, sweeper auto-closes)
// across every conversation.
//
// The client component does the actual fetch so reloads don't
// hit a server-rendered stale snapshot and the page works with
// the live WS feed in a future iteration.
export default async function OttoAuditLogPage() {
  const { currentStore } = await getServerSessionContext();
  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold text-foreground">
          Support audit log
        </h1>
        <p className="text-sm text-foreground-tertiary">
          Every significant change made to support cases in{" "}
          {currentStore?.name ?? "this store"} — who did what and when.
          Use this to audit staff actions or trace the history of a
          specific case.
        </p>
      </header>
      <OttoAuditLog />
    </div>
  );
}
