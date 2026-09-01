import { redirect } from "next/navigation";

/**
 * /settings/warehouses now lives as the "Ships from" section of
 * /settings/shipping (#177 PR 5d) — a carrier cannot quote without an
 * origin, so the two belong in one place, in dependency order.
 *
 * The route is kept rather than deleted: it shipped in 5c, the merchant's
 * nav pointed at it, and a bookmark or a link from an old email should
 * land on the section rather than a 404.
 */
export default function WarehousesSettingsPage() {
  redirect("/settings/shipping#warehouses");
}
