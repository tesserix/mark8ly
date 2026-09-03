"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { customerSignUp } from "@/app/create-account/actions";

const TRAMPOLINE_BASE =
  process.env.NEXT_PUBLIC_MARK8LY_AUTH_URL ?? "https://mark8ly.com";

// Matches apps/storefront/app/sign-in/actions.ts's AUTH_PROVIDER rule
// exactly (see that file for the full rationale): only the literal string
// "zitadel" switches this form off the GIP/Identity Toolkit path. Both
// reads must agree, since the server action rejects a Zitadel-shaped
// payload sent while the flag says GIP and vice versa.
const AUTH_PROVIDER: "gip" | "zitadel" =
  process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip";

interface GipConfig {
  apiKey: string;
  tenantId: string;
  projectId: string;
}

interface CreateAccountFormProps {
  gipConfig: GipConfig;
  storeSlug: string;
  returnUrl: string;
}

class GIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function signUpWithPassword(
  email: string,
  password: string,
  config: GipConfig,
): Promise<{ uid: string; idToken: string }> {
  if (!config.apiKey || !config.tenantId) {
    throw new GIPError(
      "config_missing",
      "Account creation is not configured for this store yet.",
    );
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=${config.apiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      returnSecureToken: true,
      tenantId: config.tenantId,
    }),
  });

  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    const code = body?.error?.message ?? "UNKNOWN";
    const friendly: Record<string, string> = {
      EMAIL_EXISTS: "An account with this email already exists. Try signing in instead.",
      WEAK_PASSWORD: "Password must be at least 6 characters.",
      INVALID_EMAIL: "Please enter a valid email address.",
      TOO_MANY_ATTEMPTS_TRY_LATER:
        "Too many attempts. Please wait a moment and try again.",
      OPERATION_NOT_ALLOWED:
        "Account creation is not enabled for this store yet.",
    };
    throw new GIPError(
      code,
      friendly[code] ?? "Account creation failed. Please try again.",
    );
  }

  const data = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: data.localId, idToken: data.idToken };
}

export function CreateAccountForm({
  gipConfig,
  storeSlug,
  returnUrl,
}: CreateAccountFormProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleGoogle() {
    if (typeof window === "undefined") return;
    const url = new URL("/auth/google", TRAMPOLINE_BASE);
    url.searchParams.set("return_to", `${window.location.origin}/account`);
    url.searchParams.set("store_slug", storeSlug);
    url.searchParams.set("intent", "signup");
    window.location.assign(url.toString());
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!email.trim()) {
      setError("Email is required.");
      return;
    }
    if (!password || password.length < 6) {
      setError("Password must be at least 6 characters.");
      return;
    }

    startTransition(async () => {
      try {
        const gipResult = await signUpWithPassword(
          email.trim(),
          password,
          gipConfig,
        );

        const result = await customerSignUp({
          idToken: gipResult.idToken,
          uid: gipResult.uid,
          storeSlug,
        });

        if (!result.ok) {
          setError(result.message);
          return;
        }

        router.push(returnUrl);
        router.refresh();
      } catch (err) {
        if (err instanceof GIPError) {
          setError(err.message);
        } else {
          setError("Something went wrong. Please try again.");
        }
      }
    });
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="mt-8 space-y-5">
      <div className="space-y-1.5">
        <label
          htmlFor="signup-email"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Email address
        </label>
        <input
          id="signup-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="signup-password"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Password
        </label>
        <input
          id="signup-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          At least 6 characters.
        </p>
      </div>

      {error && (
        <p role="alert" className="text-sm text-[color:var(--storefront-danger)]">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={pending}
        className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        {pending ? "Creating account..." : "Create account"}
      </button>

      {AUTH_PROVIDER === "gip" && (
        <>
          <div className="relative py-1">
            <div className="absolute inset-0 flex items-center" aria-hidden="true">
              <div className="w-full border-t border-[color:var(--storefront-text,var(--ink-900))]/15" />
            </div>
            <div className="relative flex justify-center">
              <span className="bg-[color:var(--storefront-background,var(--paper-200))] px-3 text-xs uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))]/55">
                or
              </span>
            </div>
          </div>

          <button
            type="button"
            onClick={handleGoogle}
            disabled={pending}
            className="inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-6 text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-text,var(--ink-900))]/40 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
              <path d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z" fill="#4285F4"/>
              <path d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" fill="#34A853"/>
              <path d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" fill="#FBBC05"/>
              <path d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z" fill="#EA4335"/>
            </svg>
            Continue with Google
          </button>
        </>
      )}

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Already have an account?{" "}
        <Link
          href="/sign-in"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Sign in
        </Link>
      </p>
    </form>
  );
}
