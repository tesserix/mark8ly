import type { AdminReviewReply } from "@/lib/api/marketplace-api";

interface ReviewReplyListProps {
  replies: AdminReviewReply[];
}

export function ReviewReplyList({ replies }: ReviewReplyListProps) {
  if (replies.length === 0) {
    return (
      <p className="text-sm text-foreground-tertiary">
        No replies yet. Be the first to respond.
      </p>
    );
  }

  return (
    <ul role="list" className="flex flex-col gap-4">
      {replies.map((reply) => (
        <li
          key={reply.id}
          className="border-l-2 border-[color:var(--moss-700)] pl-4"
        >
          <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            <span className="font-semibold text-foreground">
              {reply.author_name}
            </span>
            <span className="text-[11px] font-semibold uppercase tracking-[0.12em] text-foreground-tertiary">
              {reply.author_type}
            </span>
            <span className="text-xs text-foreground-tertiary">
              {formatDateTime(reply.created_at)}
            </span>
          </div>
          <p className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-foreground-secondary">
            {reply.content}
          </p>
        </li>
      ))}
    </ul>
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
