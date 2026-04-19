'use client'

/**
 * QueryProvider — wraps the app in a TanStack React Query context.
 *
 * The QueryClient is created once per mount via useState so that it is not
 * re-created on every server render cycle (important for Next.js App Router).
 *
 * No React Query Devtools are included: add them in a separate
 * dev-only component if needed.
 */
import { useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

interface QueryProviderProps {
  children: ReactNode
}

function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // Disable automatic refetch on window focus for admin — keeps behaviour
        // predictable and avoids unnecessary API calls in long-running sessions.
        refetchOnWindowFocus: false,
        // Retry once before surfacing an error to the UI.
        retry: 1,
      },
    },
  })
}

export function QueryProvider({ children }: QueryProviderProps) {
  const [queryClient] = useState(makeQueryClient)

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

export default QueryProvider
