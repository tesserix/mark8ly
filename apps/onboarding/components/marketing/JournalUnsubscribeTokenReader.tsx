"use client";

import type { ReactNode } from "react";
import { useSearchParams } from "next/navigation";

interface JournalUnsubscribeTokenReaderProps {
  children: (token: string | null) => ReactNode;
}

/**
 * Thin client-only wrapper around useSearchParams, kept separate from
 * JournalUnsubscribeConfirm so that component stays a plain
 * props-in/markup-out client component with no next/navigation
 * dependency — easier to unit-test (see
 * tests/unit/journal-unsubscribe-confirm.spec.tsx) and reusable if this
 * page ever needs a second consumer of the token.
 *
 * Must be rendered inside a <Suspense> boundary — useSearchParams
 * requires one during static rendering (same reasoning as
 * app/onboarding/verify/page.tsx's use of VerifyMagicLink).
 */
export function JournalUnsubscribeTokenReader({
  children,
}: JournalUnsubscribeTokenReaderProps) {
  const searchParams = useSearchParams();
  return <>{children(searchParams.get("token"))}</>;
}
