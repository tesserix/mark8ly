"use client";

// A next/link that cannot silently swallow the click (#1019).
//
// A <Link> clicked before the client router is ready has its click
// DISCARDED — not queued, not replayed, and not allowed to fall through
// to the anchor's own href. The navigation never happens and nothing is
// logged. The user clicks again and concludes the admin is janky.
//
// Measured under CDP CPU throttling, which is what a slow device or a
// loaded CI runner does to this window:
//
//   <Link href="/products/new">   0/9 at 10x, 16x, 20x
//   <a href="/products/new">      9/9
//
// and the click is LOST rather than slow: sixty seconds of waiting never
// navigates. prefetch={false} makes no difference, so it is not the
// prefetch cache, and it also happens when the anchor already carries
// __reactFiber$ and __reactProps$ — React's onClick is attached — so it
// is not "unhydrated" in the usual sense either.
//
// # Why a watchdog rather than waiting for readiness
//
// The obvious fix is to render a plain <a> until the router is ready.
// That needs a readiness signal, and there isn't one that correlates:
// neither networkidle, nor React having attached props to the anchor,
// predicts success. Only wall-clock time does.
//
// So this does not try to predict. It lets <Link> do its normal job and
// checks afterwards whether anything actually happened. If the URL has
// not moved by FALLBACK_MS, the click was one of the lost ones and we
// complete the navigation the way the browser would have.
//
// The cost in the failure case is a full page load instead of a client
// transition, which is what the user would have got from a plain <a>
// anyway — and strictly better than the nothing they get today. In the
// normal case the timer fires, sees the URL has changed, and does
// nothing.

import NextLink from "next/link";
import * as React from "react";

/**
 * How long to give the client router before assuming the click was lost.
 *
 * Generous on purpose. A client transition that is merely slow will beat
 * this, and if it does not, a hard navigation to the same href is the
 * correct destination either way — the failure mode is a slower
 * navigation, never a wrong one.
 */
const FALLBACK_MS = 700;

type SafeLinkProps = React.ComponentPropsWithoutRef<typeof NextLink>;

export function SafeLink({ href, onClick, ...rest }: SafeLinkProps) {
  const handleClick = React.useCallback(
    (event: React.MouseEvent<HTMLAnchorElement>) => {
      onClick?.(event);

      // Let the browser own anything that is not a plain left click:
      // modifier clicks and middle clicks open a new tab and leave this
      // document's URL alone, which the watchdog would misread as a lost
      // click and then navigate this tab as well.
      if (
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey ||
        (rest.target && rest.target !== "_self")
      ) {
        return;
      }

      // Only same-origin, string hrefs. A UrlObject would need the
      // router's own resolution to turn into something assignable to
      // location, and an external href is a full navigation regardless.
      if (typeof href !== "string" || !href.startsWith("/")) return;

      const before = window.location.pathname + window.location.search;
      window.setTimeout(() => {
        const now = window.location.pathname + window.location.search;
        if (now === before) window.location.href = href;
      }, FALLBACK_MS);
    },
    [href, onClick, rest.target],
  );

  return <NextLink href={href} onClick={handleClick} {...rest} />;
}
