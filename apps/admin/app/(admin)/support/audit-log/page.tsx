import { notFound } from "next/navigation";

// import { getServerSessionContext } from "@/lib/auth/serverSession";
// import { OttoAuditLog } from "@/components/support/OttoAuditLog";

// Store-wide Otto audit tail — disabled until launch.
// See docs/otto-disabled.md to re-enable.
export default async function OttoAuditLogPage() {
  notFound();

  // const { currentStore } = await getServerSessionContext();
  // return (
  //   <div className="flex flex-col gap-6 p-6">
  //     <header className="flex flex-col gap-2">
  //       <h1 className="text-2xl font-semibold text-foreground">Support audit log</h1>
  //       <p className="text-sm text-foreground-tertiary">
  //         Every significant change made to support cases in {currentStore?.name ?? "this store"}.
  //       </p>
  //     </header>
  //     <OttoAuditLog />
  //   </div>
  // );
}
