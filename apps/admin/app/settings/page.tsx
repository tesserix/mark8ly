import { redirect } from "next/navigation";

// /settings is an index route — redirect to the only real settings
// page we've shipped so far. When a second settings section lands
// this becomes a layout with a sidebar of sub-pages.
export default function SettingsPage() {
  redirect("/settings/general");
}
