import { forwardToOtto } from "../_proxy";

// Widget calls this on every mount. Otto returns the active thread for
// the caller's otto_session cookie (if any) so a page reload doesn't
// make the customer lose their chat history.
export async function GET() {
  return forwardToOtto("/api/v1/storefront/otto/resume");
}
