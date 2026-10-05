"use client";

import { useEffect } from "react";

import { captureCurrentArrival } from "@/lib/acquisition";

// Records the arrival that brought this page view, once per page load
// (#992). Renders nothing. Mounted in the root layout so the homepage,
// the comparison pages and a direct /onboarding visit all count; the
// record is read back by the signup form and sent with the session.
//
// Runs after hydration only: there is no DOM on the server and the
// capture reads window.location, document.referrer and localStorage.
// Nothing here touches OpenPanel — page views are already its job.
export function AcquisitionCapture() {
  useEffect(() => {
    captureCurrentArrival();
  }, []);
  return null;
}
