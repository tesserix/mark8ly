import { redirect } from "next/navigation";

// /settings is an index route — land on Stores (the first item in the
// grouped sidebar and where store identity lives after the general+stores
// merge).
export default function SettingsPage() {
  redirect("/settings/stores");
}
