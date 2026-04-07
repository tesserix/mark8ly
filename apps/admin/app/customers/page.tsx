import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";

export default function CustomersPage() {
  return (
    <AdminShell>
      <ComingSoon
        title="Customers"
        description="See everyone who's shopped at your store, their order history, and segments for marketing."
        eta="Later slice"
      />
    </AdminShell>
  );
}
