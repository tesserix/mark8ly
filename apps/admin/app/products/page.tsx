import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";

export default function ProductsPage() {
  return (
    <AdminShell>
      <ComingSoon
        title="Products"
        description="Add, edit, and organize everything you sell. Photos, variants, stock, pricing — all in one place."
        eta="Next slice"
      />
    </AdminShell>
  );
}
