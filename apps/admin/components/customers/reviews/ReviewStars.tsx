import { Star } from "lucide-react";

interface ReviewStarsProps {
  rating: number;
  maxRating?: number;
}

export function ReviewStars({ rating, maxRating = 5 }: ReviewStarsProps) {
  const clamped = Math.max(0, Math.min(maxRating, Math.round(rating)));
  return (
    <span
      className="inline-flex items-center gap-0.5"
      role="img"
      aria-label={`${clamped} out of ${maxRating} stars`}
    >
      {Array.from({ length: maxRating }).map((_, idx) => {
        const filled = idx < clamped;
        return (
          <Star
            key={idx}
            aria-hidden="true"
            className={`h-3.5 w-3.5 ${
              filled
                ? "fill-[color:var(--moss-700)] text-[color:var(--moss-700)]"
                : "fill-none text-foreground-tertiary"
            }`}
          />
        );
      })}
    </span>
  );
}
