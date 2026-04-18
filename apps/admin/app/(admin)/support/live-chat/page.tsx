import { notFound } from "next/navigation";

// import { getServerSessionContext } from "@/lib/auth/serverSession";
// import { LiveChatInbox } from "@/components/support/LiveChatInbox";

// Otto live-chat inbox — temporarily disabled for the public admin
// until product launch. The page returns 404 so a direct URL hit
// doesn't expose the UI either. The implementation below is left
// commented-in so re-enabling is: delete the notFound() call,
// uncomment the imports, uncomment the real return. See
// docs/otto-disabled.md.
export default async function LiveChatPage() {
  notFound();

  // const { userId, currentStore } = await getServerSessionContext();
  // return (
  //   <div className="flex flex-col gap-6 p-6">
  //     <header className="flex items-start justify-between gap-4">
  //       <div>
  //         <h1 className="text-2xl font-semibold text-foreground">Live chat</h1>
  //         <p className="mt-1 text-sm text-foreground-tertiary">
  //           Real-time support conversations from {currentStore?.name ?? "your storefront"}.
  //         </p>
  //       </div>
  //     </header>
  //     <LiveChatInbox currentUserId={userId ?? ""} />
  //   </div>
  // );
}
