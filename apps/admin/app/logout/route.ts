import { NextResponse } from "next/server";

/**
 * Logout — clears the admin session cookie and redirects to the
 * marketing site. Phase I stub: only clears the cookie on this origin.
 * Phase J will also POST to auth-bff `/auth/logout` so the session is
 * invalidated server-side (not just hidden from this browser).
 */
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";
const MARKETING_URL =
  process.env.NEXT_PUBLIC_MARKETING_URL ?? "http://localhost:4201";

export function GET() {
  const response = NextResponse.redirect(MARKETING_URL);
  response.cookies.delete(SESSION_COOKIE_NAME);
  return response;
}
