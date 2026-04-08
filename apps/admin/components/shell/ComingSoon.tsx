import Link from "next/link";
import { ArrowRight } from "lucide-react";

interface ComingSoonProps {
  title: string;
  description: string;
  eta?: string;
}

/**
 * Placeholder rendered by every stub admin route until its real
 * implementation lands. Lives inside AdminShell — see the stub page
 * files under app/{products,orders,customers,settings}.
 */
export function ComingSoon({ title, description, eta }: ComingSoonProps) {
  return (
    <div className="mx-auto max-w-4xl">
      <div className="space-y-4">
        <p className="eyebrow">Coming soon</p>
        <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground sm:text-5xl">
          {title}
        </h1>
        <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
          {description}
        </p>
        {eta && (
          <p className="text-sm text-foreground-tertiary">
            Expected:{" "}
            <span className="font-medium text-foreground">{eta}</span>
          </p>
        )}
      </div>

      <div className="mt-10 border-t border-border-subtle pt-8">
        <p className="eyebrow">Why this exists</p>
        <div className="mt-3 max-w-2xl space-y-3 text-sm leading-7 text-foreground-secondary">
          <p>
            The route is already wired so the information architecture stays
            stable while features land.
          </p>
          <p>
            New slices can plug into a finished shell instead of being
            redesigned each time.
          </p>
        </div>
        <div className="mt-8">
          <Link
            href="/dashboard"
            className="inline-flex h-12 items-center gap-2 rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover"
          >
            Back to dashboard
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </Link>
        </div>
      </div>
    </div>
  );
}
