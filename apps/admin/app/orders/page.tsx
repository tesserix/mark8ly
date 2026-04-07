import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";

export default function OrdersPage() {
  return (
    <AdminShell>
      <ComingSoon
        title="Orders"
        description="When customers start buying, every order lands here with status, items, shipping, and refund controls."
        eta="Next slice"
      />
    </AdminShell>
  );
}
