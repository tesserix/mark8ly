import Link from "next/link";

export function SlimFooter() {
  return (
    <footer className="border-t border-warm-200/80 bg-background/70 backdrop-blur-sm">
      <div className="max-w-6xl mx-auto flex flex-col gap-3 px-6 py-4 text-sm text-foreground-secondary sm:flex-row sm:items-center sm:justify-between">
        <p>
          Need help?{" "}
          <a
            href="mailto:support@mark8ly.com"
            className="font-medium text-foreground hover:text-terracotta-700 transition-colors"
          >
            support@mark8ly.com
          </a>
        </p>

        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <Link
            href="/terms"
            className="hover:text-foreground transition-colors"
          >
            Terms
          </Link>
          <Link
            href="/privacy"
            className="hover:text-foreground transition-colors"
          >
            Privacy
          </Link>
          <Link
            href="/legal"
            className="hover:text-foreground transition-colors"
          >
            Security
          </Link>
        </div>
      </div>
    </footer>
  );
}
