import { redirect } from "next/navigation";

/**
 * /settings/general — legacy route. General store identity now lives on
 * /settings/stores (merged so the multi-store list and the active store's
 * editable fields share one page). Redirect to preserve bookmarks.
 */
export default function GeneralSettingsRedirect() {
  redirect("/settings/stores");
}
