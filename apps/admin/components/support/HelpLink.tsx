interface HelpLinkProps {
  slug: string;
  label?: string;
}

export function HelpLink({ slug, label = "Learn more" }: HelpLinkProps) {
  return (
    <a
      href={`/support/help/${slug}`}
      className="inline-flex min-h-[44px] items-center text-sm text-[color:var(--moss-700)] hover:underline"
    >
      {label} &rarr;
    </a>
  );
}
