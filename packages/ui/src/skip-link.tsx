interface SkipLinkProps {
  /** id of the <main> element to focus when activated */
  targetId?: string;
  /** Visible label */
  label?: string;
}

/**
 * SkipLink — first focusable element in <body>. Hidden from
 * sighted users until it receives keyboard focus, then jumps
 * the user past the header into the main content.
 *
 * Required for WCAG 2.4.1 (Bypass Blocks).
 *
 * Implementation: pure Tailwind `sr-only` + `focus:not-sr-only`.
 * Zero JS, zero arbitrary-value classes, zero dependency on
 * the consuming app having a particular @source configuration.
 * `sr-only` and its focus variant are baseline Tailwind utilities
 * that ship in every build.
 */
export function SkipLink({
  targetId = "main",
  label = "Skip to main content",
}: SkipLinkProps) {
  return (
    <a
      href={`#${targetId}`}
      className="
        sr-only
        focus:not-sr-only
        focus:fixed focus:left-3 focus:top-3 focus:z-50
        focus:rounded-md focus:bg-ink-900 focus:px-5 focus:py-3
        focus:text-base focus:font-medium focus:text-paper-50
        focus:no-underline focus:shadow-lg
      "
    >
      {label}
    </a>
  );
}
