import { redirect } from "next/navigation";

/**
 * Legacy subscription route — kept alive only to redirect old
 * bookmarks to the v2.3 billing dashboard. The v2.3 UI is at
 * `/settings/billing` (built in P16) and owns plan state, portal
 * access, payment method, and white-label app status.
 *
 * This route will be removed entirely once analytics show no
 * traffic here.
 */
export default function SubscriptionRedirect(): never {
  redirect("/settings/billing");
}
