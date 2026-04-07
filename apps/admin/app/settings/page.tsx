import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";

export default function SettingsPage() {
  return (
    <AdminShell>
      <ComingSoon
        title="Settings"
        description="Store details, payments, shipping, tax, domains, and roles. This is where you fine-tune everything."
        eta="Next slice"
      />
    </AdminShell>
  );
}
