import Link from "next/link";
import { ArrowRight } from "lucide-react";

interface ComingSoonProps {
  title: string;
  description: string;
  eta?: string;
}

/**
 * Placeholder rendered by every stub admin route until its real
 * implementation lands. Phase I uses this for /products, /orders,
 * /customers, /settings — every top-level nav item other than the
 * dashboard.
 *
 * Keeping a dedicated component (instead of ad-hoc inline markup on
 * each stub) means the "what's coming next" messaging is consistent
 * across the whole app and updates in one place when we ship the real
 * feature.
 */
export function ComingSoon({ title, description, eta }: ComingSoonProps) {
  return (
    <div className="mx-auto max-w-2xl">
      <div className="rounded-[2rem] border border-warm-200/90 bg-white/90 p-10 shadow-[0_20px_60px_rgba(43,38,34,0.08)]">
        <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Coming soon
        </p>
        <h1 className="mt-3 font-serif text-3xl font-medium tracking-tight text-foreground">
          {title}
        </h1>
        <p className="mt-4 text-base leading-7 text-foreground-secondary">
          {description}
        </p>
        {eta && (
          <p className="mt-4 text-sm text-foreground-tertiary">
            Expected: <span className="font-medium text-foreground">{eta}</span>
          </p>
        )}
        <div className="mt-8 flex items-center gap-3">
          <Link
            href="/dashboard"
            className="inline-flex items-center gap-2 rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-md"
          >
            Back to dashboard
            <ArrowRight className="h-4 w-4" />
          </Link>
        </div>
      </div>
    </div>
  );
}
