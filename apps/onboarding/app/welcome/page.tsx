import { PostSubmitShell } from "@/components/onboarding/PostSubmitShell";

// Welcome page shown after successful onboarding + auto-login.
//
// In production this would redirect to the merchant's admin app at
// {slug}-admin.mark8ly.com. For Phase F we render a simple confirmation
// — admin app porting is a future phase.

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
            Your store is live and you&apos;re signed in. The admin dashboard is
            landing in the next phase, but your storefront foundation is ready.
          </p>
        </div>
      </div>
    </PostSubmitShell>
  );
}
