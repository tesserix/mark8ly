import { GoogleAuthTrampoline } from "./GoogleAuthTrampoline";

// Rendered per request so middleware's CSP nonce reaches the script
// tags; a prerendered page under that policy would have them blocked.
// Route segment config is ignored in a "use client" file, so the client
// body lives in GoogleAuthTrampoline.tsx and this stays a server page.
export const dynamic = "force-dynamic";

export default function GoogleAuthTrampolinePage() {
  return <GoogleAuthTrampoline />;
}
