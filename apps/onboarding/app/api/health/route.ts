import { NextResponse } from 'next/server'

// Kubelet liveness/readiness probe target. Mirrors the admin/storefront
// pattern — cheap static handler, no upstream calls.
export const dynamic = 'force-static'
export const revalidate = false

export function GET(): NextResponse {
  return NextResponse.json({ status: 'ok' }, { status: 200 })
}
