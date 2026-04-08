import { SignInForm } from "@/components/auth/SignInForm";

export const metadata = { title: "Sign in — Mark8ly" };

/**
 * Admin /login — Phase M.
 *
 * The returning-user funnel lives here, not on the marketing site.
 * Two paths to a session:
 *   1. Email + password — Identity Toolkit signInWithPassword + the
 *      `signIn` server action which looks up workspace_tenant by GIP
 *      UID and calls auth-bff /auth/auto-login.
 *   2. Continue with Google — gsi/client popup, exchanged via Identity
 *      Toolkit signInWithIdp, then the same server action.
 */
export default function LoginPage() {
  return (
    <main className="min-h-screen px-4 py-10 sm:px-6 sm:py-14">
      <div className="mx-auto grid min-h-[calc(100vh-5rem)] max-w-6xl items-center gap-8 lg:grid-cols-[minmax(0,1.1fr)_minmax(26rem,0.9fr)]">
        <section className="hidden lg:block">
          <p className="admin-eyebrow">mark8ly admin</p>
          <h1 className="mt-4 max-w-xl font-serif text-6xl font-medium leading-none tracking-tight text-foreground">
            Your store deserves a calmer control room.
          </h1>
          <p className="mt-6 max-w-xl text-lg leading-8 text-muted-foreground">
            Sign in to manage your catalog, orders, and storefront settings from a
            workspace designed to stay fast, focused, and readable.
          </p>

          <div className="mt-10 grid max-w-2xl gap-4 sm:grid-cols-3">
            {loginHighlights.map((item) => (
              <div key={item.title} className="admin-panel rounded-[1.5rem] p-5">
                <p className="text-sm font-semibold text-foreground">{item.title}</p>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{item.body}</p>
              </div>
            ))}
          </div>
        </section>

        <SignInForm />
      </div>
    </main>
  );
}

const loginHighlights = [
  {
    title: "One workspace",
    body: "Products, orders, customers, and settings all live inside the same shell.",
  },
  {
    title: "Same store identity",
    body: "Your admin stays tied to the store you created during onboarding.",
  },
  {
    title: "Built for growth",
    body: "The migrated surface is ready for the next feature slices without a redesign later.",
  },
];
