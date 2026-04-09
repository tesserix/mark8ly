import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@repo/ui/coming-soon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function OrdersPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="Orders"
        description="When customers start buying, every order lands here with status, items, shipping, and refund controls."
        eta="Next slice"
      />
    </AdminShell>
  );
}
