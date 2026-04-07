import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function ProductsPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="Products"
        description="Add, edit, and organize everything you sell. Photos, variants, stock, pricing — all in one place."
        eta="Next slice"
      />
    </AdminShell>
  );
}
