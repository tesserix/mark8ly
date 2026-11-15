// A cashier (#1104) sells in store and nothing else, so the only admin page
// they reach is the sale screen. The API refuses them everywhere else; this
// keeps them from landing on pages that would only show errors.
export function cashierPath(pathname: string): "allow" | "redirect" {
  if (pathname === "/sell" || pathname.startsWith("/sell/")) return "allow";
  if (pathname === "/logout") return "allow";
  return "redirect";
}
