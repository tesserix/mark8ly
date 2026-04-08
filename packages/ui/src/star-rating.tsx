interface StarRatingProps {
  /** Rating value, 0-5. Halves supported. */
  value: number;
  /** Total stars in the scale, defaults to 5. */
  outOf?: number;
  /** Pixel size of each star. */
  size?: number;
  /** Optional className for layout (Tailwind passthrough). */
  className?: string;
}

/**
 * StarRating — accessible rating display.
 *
 * Uses a single <span role="img" aria-label="..."> wrapper so
 * screen readers announce the rating once ("Rated 4.9 out of
 * 5 stars") instead of reading five star characters. Inner SVGs
 * are aria-hidden.
 *
 * Color comes from currentColor — set it on the parent (e.g.
 * `text-moss-700` or `text-ink-900`) so the widget honors the
 * design system instead of forcing an off-palette amber.
 */
export function StarRating({
  value,
  outOf = 5,
  size = 16,
  className,
}: StarRatingProps) {
  const label = `Rated ${value} out of ${outOf} stars`;
  const stars = Array.from({ length: outOf }, (_, i) => {
    const fill = Math.max(0, Math.min(1, value - i));
    return fill;
  });

  return (
    <span
      role="img"
      aria-label={label}
      className={className}
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: "2px",
        color: "currentColor",
      }}
    >
      {stars.map((fill, i) => (
        <Star key={i} size={size} fill={fill} />
      ))}
    </span>
  );
}

interface StarProps {
  size: number;
  /** 0 = empty, 1 = full, fractional = partial fill */
  fill: number;
}

function Star({ size, fill }: StarProps) {
  const id = `star-clip-${Math.random().toString(36).slice(2, 9)}`;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <defs>
        <clipPath id={id}>
          <rect x="0" y="0" width={24 * fill} height="24" />
        </clipPath>
      </defs>
      <path
        d="M12 2.5l2.9 6.4 6.6.7-5 4.7 1.4 6.7L12 17.5l-5.9 3.5L7.5 14.3l-5-4.7 6.6-.7L12 2.5z"
        stroke="currentColor"
        strokeWidth="1.4"
        strokeLinejoin="round"
      />
      {fill > 0 && (
        <path
          d="M12 2.5l2.9 6.4 6.6.7-5 4.7 1.4 6.7L12 17.5l-5.9 3.5L7.5 14.3l-5-4.7 6.6-.7L12 2.5z"
          fill="currentColor"
          clipPath={`url(#${id})`}
        />
      )}
    </svg>
  );
}
