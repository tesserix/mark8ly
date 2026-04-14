import { NextResponse, type NextRequest } from "next/server";

// Captures `?ref=CODE` from any page load and persists it in the
// `mp_referral` cookie (30 days). Enrollment actions read this cookie
// so the referral attribution survives the gap between a friend clicking
// an invite link and actually signing up + enrolling in the loyalty
// program.
export function middleware(req: NextRequest) {
  const ref = req.nextUrl.searchParams.get("ref");
  if (!ref || !/^[A-Z0-9]{4,20}$/.test(ref)) {
    return NextResponse.next();
  }

  const res = NextResponse.next();
  res.cookies.set({
    name: "mp_referral",
    value: ref,
    path: "/",
    httpOnly: true,
    sameSite: "lax",
    maxAge: 60 * 60 * 24 * 30,
  });
  return res;
}

export const config = {
  // Skip Next internals, API routes (they don't need the cookie set
  // by middleware — server actions read it directly), and static assets.
  matcher: ["/((?!_next|api|.*\\.(?:ico|png|jpg|jpeg|svg|webp|css|js|woff2?)).*)"],
};
