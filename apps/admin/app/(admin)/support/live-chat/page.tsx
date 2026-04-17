import { getServerSessionContext } from "@/lib/auth/serverSession";
import { LiveChatInbox } from "@/components/support/LiveChatInbox";

// Otto live-chat inbox — where support staff accept, reply to, and close
// real-time customer conversations. The heavy lifting is in the reusable
// OttoInbox component from @repo/otto-widget; this page just wires up the
// session context and mounts the client component.
export default async function LiveChatPage() {
  const { userId, currentStore } = await getServerSessionContext();

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-foreground">
            Live chat
          </h1>
          <p className="mt-1 text-sm text-foreground-tertiary">
            Real-time support conversations from{" "}
            {currentStore?.name ?? "your storefront"}. Customers appear here
            the moment they send a message.
          </p>
        </div>
      </header>

      <LiveChatInbox currentUserId={userId ?? ""} />
    </div>
  );
}
