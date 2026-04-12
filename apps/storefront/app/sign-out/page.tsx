import { cookies } from "next/headers";
import { redirect } from "next/navigation";

/**
 * /sign-out — clears the customer session cookie and redirects home.
 * Visiting this URL (GET) is enough; no form or button needed.
 */
export default async function SignOutPage() {
  const c = await cookies();
  c.delete("mp_customer_session");
  redirect("/");
}
