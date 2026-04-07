import { redirect } from "next/navigation";

// The root of the admin app is always a redirect to /dashboard. The real
// auth gate lives in middleware.ts — by the time the browser sees this
// page it has already been validated. Anonymous visitors are redirected
// to the marketing /login page by the middleware before they get here.
export default function AdminRoot() {
  redirect("/dashboard");
}
