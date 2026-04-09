import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@repo/ui/coming-soon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function CustomersPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="Customers"
        description="See everyone who's shopped at your store, their order history, and segments for marketing."
        eta="Later slice"
      />
    </AdminShell>
  );
}
