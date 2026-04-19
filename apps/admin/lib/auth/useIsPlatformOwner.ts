/**
 * useIsPlatformOwner — client-side hook that reads whether the current
 * session belongs to a Tesserix platform super-admin (CSM / internal team).
 *
 * The platform-owner flag is injected by middleware as the
 * `x-session-is-platform-owner` header when the GIP token carries the
 * `platform_owner: true` claim (set on the tesserix-platform GIP tenant).
 *
 * Until P15 / auth-bff lands the claim, this hook reads a context value
 * seeded by the admin shell layout. The safe default is `false` — platform
 * tooling is never accidentally shown to merchants.
 *
 * TODO(P15): once the `x-session-is-platform-owner` middleware header is
 * wired, promote this to read it directly from the shell context rather
 * than relying on a prop drill.
 */
'use client'

import { createContext, useContext } from 'react'

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

export const PlatformOwnerContext = createContext<boolean>(false)

/**
 * Returns true when the current user is a Tesserix platform super-admin.
 *
 * Use `<PlatformOwnerContext.Provider value={isPlatformOwner}>` in the
 * admin shell layout to seed the value from the server session.
 */
export function useIsPlatformOwner(): boolean {
  return useContext(PlatformOwnerContext)
}
