// Welcome page shown after successful onboarding + auto-login.
//
// In production this would redirect to the merchant's admin app at
// {slug}-admin.mark8ly.com. For Phase F we render a simple confirmation
// — admin app porting is a future phase.

export default function WelcomePage() {
  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <div className="max-w-md text-center">
        <div className="text-6xl mb-6">🎉</div>
        <h1 className="text-3xl font-semibold tracking-tight">
          Welcome to Mark8ly
        </h1>
        <p className="mt-4 text-zinc-600 dark:text-zinc-400">
          Your store is live and you're signed in. The admin dashboard is
          coming in the next phase.
        </p>
      </div>
    </main>
  );
}
