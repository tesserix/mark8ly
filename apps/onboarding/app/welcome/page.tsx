import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";

// Welcome page shown after successful onboarding + auto-login. The
// session cookie has already been minted by the time the user lands
// here, so the "Open admin dashboard" CTA below works immediately —
// no extra sign-in step.

const ADMIN_URL =
  process.env.NEXT_PUBLIC_ADMIN_URL ?? "http://localhost:4202/dashboard";

export default function WelcomePage() {
  return (
    <PostSubmitShell
      eyebrow="Store ready"
      title="You made it live."
      description="The final screen should feel like an arrival moment, with the same warmth and authority as the onboarding experience that led to it."
    >
      <div className="w-full max-w-2xl overflow-hidden rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm">
        <div className="px-8 py-10 text-center">
          <div className="mx-auto flex h-18 w-18 items-center justify-center rounded-full border border-sage-200 bg-sage-50 text-4xl shadow-sm">
            ✦
          </div>
          <p className="mt-5 text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
            Store ready
          </p>
          <h1 className="mt-2 font-serif text-4xl font-medium tracking-tight text-foreground">
            Welcome to Mark8ly
          </h1>
          <p className="mt-4 text-base leading-7 text-foreground-secondary">
            Your store is live and you&apos;re signed in. Head into the admin
            dashboard to add your first product, customize your storefront,
            and explore your settings.
          </p>
          <div className="mt-8 flex flex-col items-center gap-3 sm:flex-row sm:justify-center">
            <a
              href={ADMIN_URL}
              className="inline-flex items-center gap-2 rounded-full bg-foreground px-6 py-3 text-sm font-medium text-background shadow-[0_14px_28px_rgba(31,30,28,0.18)] transition-[background-color,box-shadow] hover:bg-foreground/90 hover:shadow-[0_18px_34px_rgba(31,30,28,0.22)]"
            >
              Open admin dashboard
              <span aria-hidden>→</span>
            </a>
            <a
              href="/"
              className="text-sm font-medium text-foreground-secondary hover:text-foreground"
            >
              Back to home
            </a>
          </div>
        </div>
      </div>
    </PostSubmitShell>
  );
}
