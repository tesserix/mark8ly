"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { customerSignIn } from "@/app/sign-in/actions";

interface GipConfig {
  apiKey: string;
  tenantId: string;
  projectId: string;
  googleClientId: string;
}

interface CustomerSignInFormProps {
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

async function signInWithPassword(
  email: string,
  password: string,
  config: GipConfig,
): Promise<{ uid: string; idToken: string }> {
  if (!config.apiKey || !config.tenantId) {
    throw new GIPError(
      "config_missing",
      "Sign-in is not configured for this store yet.",
    );
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${config.apiKey}`;
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
      EMAIL_NOT_FOUND: "No account found with this email.",
      INVALID_PASSWORD: "Incorrect password.",
      INVALID_LOGIN_CREDENTIALS: "Email or password is incorrect.",
      USER_DISABLED: "This account has been disabled.",
      TOO_MANY_ATTEMPTS_TRY_LATER:
        "Too many attempts. Please wait a moment and try again.",
    };
    throw new GIPError(code, friendly[code] ?? "Sign-in failed. Please try again.");
  }

  const data = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: data.localId, idToken: data.idToken };
}

export function CustomerSignInForm({
  gipConfig,
  storeSlug,
  returnUrl,
}: CustomerSignInFormProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!email.trim()) {
      setError("Email is required.");
      return;
    }
    if (!password) {
      setError("Password is required.");
      return;
    }

    startTransition(async () => {
      try {
        const gipResult = await signInWithPassword(
          email.trim(),
          password,
          gipConfig,
        );

        const result = await customerSignIn({
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
          htmlFor="customer-email"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Email address
        </label>
        <input
          id="customer-email"
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
          htmlFor="customer-password"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Password
        </label>
        <input
          id="customer-password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
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
        {pending ? "Signing in..." : "Sign in"}
      </button>

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Don&apos;t have an account?{" "}
        <Link
          href="/create-account"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Create one
        </Link>
      </p>
    </form>
  );
}
