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
    <div className="mx-auto max-w-2xl">
      <div className="rounded-2xl border border-border bg-card p-10 shadow-sm">
        <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
          Coming soon
        </p>
        <h1 className="mt-3 font-serif text-3xl font-medium tracking-tight text-card-foreground">
          {title}
        </h1>
        <p className="mt-4 text-base leading-7 text-muted-foreground">
          {description}
        </p>
        {eta && (
          <p className="mt-4 text-sm text-muted-foreground">
            Expected:{" "}
            <span className="font-medium text-card-foreground">{eta}</span>
          </p>
        )}
        <div className="mt-8 flex items-center gap-3">
          <Link
            href="/dashboard"
            className="inline-flex items-center gap-2 rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-sm transition-[background-color,box-shadow] hover:opacity-90 hover:shadow-md"
          >
            Back to dashboard
            <ArrowRight className="h-4 w-4" />
          </Link>
        </div>
      </div>
    </div>
  );
}
