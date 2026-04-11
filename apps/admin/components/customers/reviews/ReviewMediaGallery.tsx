import type { AdminReviewMedia } from "@/lib/api/marketplace-api";

interface ReviewMediaGalleryProps {
  media: AdminReviewMedia[];
}

export function ReviewMediaGallery({ media }: ReviewMediaGalleryProps) {
  if (media.length === 0) return null;

  return (
    <section
      aria-labelledby="review-media-heading"
      className="flex flex-col gap-3"
    >
      <h2
        id="review-media-heading"
        className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Media
      </h2>
      <ul
        role="list"
        className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4"
      >
        {media.map((item) => (
          <li
            key={item.id}
            className="relative aspect-square overflow-hidden rounded-sm border border-border-subtle bg-paper-100"
          >
            {item.media_type === "video" ? (
              // eslint-disable-next-line jsx-a11y/media-has-caption
              <video
                src={item.url}
                controls
                className="h-full w-full object-cover"
              />
            ) : (
              // Use a plain img so we don't need next/image remotePatterns
              // configured for every review upload host.
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={item.url}
                alt={item.alt ?? "Review media"}
                className="h-full w-full object-cover"
              />
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
