import Link from "next/link";
import { ChevronLeft } from "lucide-react";

interface BackLinkProps {
  href: string;
  children: React.ReactNode;
  className?: string;
}

export function BackLink({ href, children, className }: BackLinkProps) {
  return (
    <Link
      href={href}
      className={
        "inline-flex min-h-[32px] items-center gap-1 text-sm text-foreground-secondary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] focus-visible:rounded-sm" +
        (className ? ` ${className}` : "")
      }
    >
      <ChevronLeft className="h-4 w-4" aria-hidden="true" />
      {children}
    </Link>
  );
}
