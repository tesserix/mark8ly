import { StubPage } from "@/components/marketing/StubPage";

export const metadata = { title: "Sign in — Mark8ly" };

/**
 * Marketing /login — landing page the admin middleware bounces to
 * when the session cookie is missing or invalid. Phase J stub: tells
 * the visitor how to get back in (start a new store, or wait for the
 * passwordless sign-in flow we haven't built yet).
 *
 * A real "enter your email, we'll send a magic link" sign-in form
 * lands in a later slice. For now this page's job is just to exist
 * so admin redirects land somewhere real instead of 404-ing.
 */
export default function LoginPage() {
  return (
    <StubPage
      title="Sign in"
      description="Passwordless sign-in for existing stores is on the way. In the meantime, start a new store from the home page and we'll email you a link."
    />
  );
}
