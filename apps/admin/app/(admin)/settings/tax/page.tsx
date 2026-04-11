import { redirect } from "next/navigation";

/**
 * /settings/tax — legacy route. Tax configuration is country-driven and now
 * folded into /settings/payments (the "Payments & tax" page). Redirect to
 * preserve bookmarks.
 */
export default function TaxSettingsRedirect() {
  redirect("/settings/payments");
}
