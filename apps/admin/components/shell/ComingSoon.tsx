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
      <div className="admin-surface rounded-[2rem] p-6 sm:p-8">
        <div className="grid gap-8 lg:grid-cols-[minmax(0,1.15fr)_minmax(16rem,0.7fr)]">
          <div>
            <p className="admin-eyebrow">Coming Soon</p>
            <h1 className="mt-3 font-serif text-3xl font-medium tracking-tight text-card-foreground sm:text-4xl">
              {title}
            </h1>
            <p className="mt-4 max-w-2xl text-base leading-7 text-muted-foreground">
              {description}
            </p>
            {eta && (
              <p className="mt-5 text-sm text-muted-foreground">
                Expected:{" "}
                <span className="font-medium text-card-foreground">{eta}</span>
              </p>
            )}
            <div className="mt-8 flex items-center gap-3">
              <Link
                href="/dashboard"
                className="inline-flex items-center gap-2 rounded-full bg-primary px-5 py-3 text-sm font-medium text-primary-foreground shadow-[0_16px_36px_rgba(34,28,23,0.18)] transition-[transform,box-shadow,opacity] hover:-translate-y-0.5 hover:opacity-95"
              >
                Back to dashboard
                <ArrowRight className="h-4 w-4" />
              </Link>
            </div>
          </div>

          <div className="admin-panel rounded-[1.6rem] p-5">
            <p className="admin-eyebrow">Why This Exists</p>
            <div className="mt-5 space-y-4 text-sm leading-6 text-muted-foreground">
              <p>The route is already wired so the information architecture stays stable while features land.</p>
              <p>That means new slices can plug into a finished shell instead of being redesigned each time.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
