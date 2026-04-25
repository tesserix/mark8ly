import { NextResponse } from "next/server";

// Kubelet liveness/readiness probe target. Stays cheap: no DB, no
// auth-bff round-trip — the surrounding deployment manifest probes
// here precisely because hitting `/` redirects unauthenticated traffic
// to /login (a 30x), which kubelet logs as "Probe terminated redirects"
// every period.
export const dynamic = "force-static";
export const revalidate = false;

export function GET() {
  return NextResponse.json({ status: "ok" }, { status: 200 });
}
