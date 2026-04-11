import { redirect } from "next/navigation";

// Support is a section, not a page — its sidebar entry expands into
// Tickets and Help Center. Direct navigation to `/support` (typed
// URL, bookmark, stale prefetch) gets forwarded to the tickets list
// so users don't hit a 404.
export default function SupportIndexPage(): never {
  redirect("/support/tickets");
}
