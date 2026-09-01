import type { ReactNode } from "react";

/* ============================================================
   MailLink — the only way a `mailto:` anchor should be rendered
   on any Mark8ly surface. See GitHub issue #147.

   Cloudflare's "Email Address Obfuscation" feature rewrites every
   literal `mailto:` link in the *served* HTML into
   `<a href="/cdn-cgi/l/email-protection#...">`. On mark8ly.com that
   path 404s — the Cloudflare Tunnel forwards /cdn-cgi straight to
   the Next.js origin, which has no such route. That means:

     1. Googlebot crawls a 404 off every legal/contact page (the GSC
        "Not found (404)" report).
     2. Worse: a real visitor clicking the address gets a 404 page
        instead of their mail client. Live UX bug, not just SEO.

   Cloudflare's obfuscation officially honours an opt-out marker:
   anything between the literal HTML comments `<!--email_off-->`
   and `<!--email_on-->` is left untouched. React can't render bare
   comment nodes, so we emit them as the innerHTML of invisible
   `<span>` wrappers either side of the anchor. Do NOT delete these
   spans as "dead markup" — they are the entire fix.

   Lives in @repo/ui because the bug is a property of the Cloudflare
   edge, not of any one app: it applies to every public surface we
   serve through that edge. Consumers today are the onboarding
   marketing site and admin's public /pricing page.

   Do not render a raw `mailto:` anchor anywhere else; always go
   through this component instead.
   ============================================================ */

interface MailLinkProps {
  email: string;
  className?: string;
  children?: ReactNode;
}

export function MailLink({ email, className, children }: MailLinkProps) {
  return (
    <>
      <span aria-hidden dangerouslySetInnerHTML={{ __html: "<!--email_off-->" }} />
      <a href={`mailto:${email}`} className={className}>
        {children ?? email}
      </a>
      <span aria-hidden dangerouslySetInnerHTML={{ __html: "<!--email_on-->" }} />
    </>
  );
}
