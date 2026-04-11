// Barrel for the admin layout primitives.
//
// Only server-safe exports live here — every page in the app is a server
// component and imports from this barrel. Re-exporting a `"use client"`
// component (e.g. `ErrorState`) would force the RSC compiler to treat the
// whole barrel as a client boundary, and any server-side import from it
// would resolve `undefined` at prerender time.
//
// Client components that need editorial chrome (route `error.tsx` files)
// must import `ErrorState` directly from its own module:
//     import { ErrorState } from "@/components/layout/ErrorState";

export { AdminPage, PageSection, ReadOnlyNotice } from "./AdminPage";
export { PageSkeleton } from "./PageSkeleton";
