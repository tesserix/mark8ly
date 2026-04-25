import { NextResponse } from "next/server";

// Kubelet liveness/readiness probe target. Cheap, no upstream calls —
// SSR'ing `/` was timing out under cold-start (1s default probe
// timeout) and triggering false-positive restarts.
export const dynamic = "force-static";
export const revalidate = false;

export function GET() {
  return NextResponse.json({ status: "ok" }, { status: 200 });
}
