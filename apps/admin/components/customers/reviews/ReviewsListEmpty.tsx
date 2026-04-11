interface ReviewsListEmptyProps {
  variant: "no-reviews" | "no-matches";
  clearFiltersHref?: string;
}

export function ReviewsListEmpty({
  variant,
  clearFiltersHref,
}: ReviewsListEmptyProps) {
  const title =
    variant === "no-reviews"
      ? "No reviews yet"
      : "No reviews match your filters";
  const body =
    variant === "no-reviews"
      ? "Once customers start leaving product reviews, you'll see them here for moderation."
      : "Try a different status filter to find the reviews you're looking for.";

  return (
    <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-16">
      <h2 className="font-serif text-xl text-foreground">{title}</h2>
      <p className="max-w-md text-sm text-foreground-secondary">{body}</p>
      {clearFiltersHref && (
        <a
          href={clearFiltersHref}
          className="mt-2 inline-flex min-h-[2.75rem] items-center text-sm text-[color:var(--moss-700)] underline-offset-4 hover:underline"
        >
          Clear filters
        </a>
      )}
    </div>
  );
}
