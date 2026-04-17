import type { AdminReviewReply } from "@/lib/api/marketplace-api";

interface ReviewReplyListProps {
  replies: AdminReviewReply[];
}

// Renders the replies as a two-level tree — top-level comments/merchant
// replies first, each with their nested children indented. Keeps the
// admin moderation view scannable at a glance while faithfully showing
// the structure the customer sees on the storefront.
export function ReviewReplyList({ replies }: ReviewReplyListProps) {
  if (replies.length === 0) {
    return (
      <p className="text-sm text-foreground-tertiary">
        No replies yet. Be the first to respond.
      </p>
    );
  }

  const topLevel: AdminReviewReply[] = [];
  const children = new Map<string, AdminReviewReply[]>();
  for (const r of replies) {
    const parent = r.parent_reply_id ?? null;
    if (!parent) {
      topLevel.push(r);
    } else {
      const arr = children.get(parent) ?? [];
      arr.push(r);
      children.set(parent, arr);
    }
  }
  const byCreatedAt = (a: AdminReviewReply, b: AdminReviewReply) =>
    a.created_at.localeCompare(b.created_at);
  topLevel.sort(byCreatedAt);
  children.forEach((arr) => arr.sort(byCreatedAt));

  return (
    <ul role="list" className="flex flex-col gap-4">
      {topLevel.map((reply) => (
        <li key={reply.id}>
          <ReplyCard reply={reply} />
          {(children.get(reply.id) ?? []).length > 0 && (
            <ul role="list" className="mt-3 flex flex-col gap-3 pl-5">
              {(children.get(reply.id) ?? []).map((child) => (
                <li key={child.id}>
                  <ReplyCard reply={child} nested />
                </li>
              ))}
            </ul>
          )}
        </li>
      ))}
    </ul>
  );
}

function ReplyCard({
  reply,
  nested = false,
}: {
  reply: AdminReviewReply;
  nested?: boolean;
}) {
  const isMerchant = reply.author_type === "merchant";
  const accent = isMerchant
    ? "border-[color:var(--moss-700)]"
    : "border-[color:var(--paper-300)]";
  const bg = nested ? "bg-[color:var(--background-elevated)]" : "";
  return (
    <div className={`border-l-2 ${accent} ${bg} pl-4`}>
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="font-semibold text-foreground">
          {reply.author_name}
        </span>
        <span
          className={`text-[10px] font-semibold uppercase tracking-[0.12em] ${
            isMerchant
              ? "rounded-full bg-[color:var(--moss-700)]/15 px-2 py-0.5 text-[color:var(--moss-700)]"
              : "rounded-full bg-foreground/10 px-2 py-0.5 text-foreground-tertiary"
          }`}
        >
          {isMerchant ? "Store team" : "Customer"}
        </span>
        <span className="text-xs text-foreground-tertiary">
          {formatDateTime(reply.created_at)}
        </span>
      </div>
      <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-foreground-secondary">
        {reply.content}
      </p>
    </div>
  );
}

function formatDateTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}
