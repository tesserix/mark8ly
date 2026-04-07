import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function SettingsPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="Settings"
        description="Store details, payments, shipping, tax, domains, and roles. This is where you fine-tune everything."
        eta="Next slice"
      />
    </AdminShell>
  );
}
