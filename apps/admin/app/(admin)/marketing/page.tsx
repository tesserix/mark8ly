import { redirect } from "next/navigation";

// Marketing is a section, not a page — its sidebar entry expands a
// submenu (Campaigns, Coupons, Gift Cards, Loyalty, Segments). Anyone
// who lands on `/marketing` directly — typed URL, bookmark, or a
// prefetch from a stale build — gets routed to the Campaigns list so
// they don't see a 404.
export default function MarketingIndexPage(): never {
  redirect("/marketing/campaigns");
}
