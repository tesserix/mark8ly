// Official App Store / Google Play badges linking to the Mark8ly Admin
// mobile app, shared by the onboarding success page and the admin.
//
// No hooks here on purpose — this stays usable inside React Server
// Components. The dismissible dashboard prompt wraps it in a client
// component of its own.

export interface AppStoreLinks {
  /** Canonical App Store product URL. Empty string ⇒ badge is not rendered. */
  readonly ios: string;
  /** Canonical Play Store product URL. Empty string ⇒ badge is not rendered. */
  readonly android: string;
}

/**
 * Single source of truth for the store URLs. Each platform renders only
 * when its URL is non-empty, so a platform can be switched on without
 * touching any of the consuming surfaces.
 *
 * 🔴 iOS IS INTENTIONALLY EMPTY. mobile-admin 1.0.0 is an *initial* release
 * still in App Store review, and apps.apple.com product URLs do not resolve
 * until first release — shipping a badge now would be a dead link. Fill this
 * in the moment the app is live; get the URL from the store itself rather
 * than hand-copying it out of App Store Connect:
 *
 *   curl -s "https://itunes.apple.com/lookup?bundleId=com.mark8ly.admin"
 *
 * A `resultCount` of 0 means still-not-live. Use the canonical
 * `https://apps.apple.com/app/apple-store/id<trackId>` form, with no locale
 * prefix, so it resolves for merchants in every market.
 */
export const MOBILE_ADMIN_APP_LINKS: AppStoreLinks = {
  ios: "",
  android: "https://play.google.com/store/apps/details?id=com.mark8ly.admin",
};

// Badge artwork lives in each consuming app's public/badges/ directory.
// Both files are the unmodified official assets — Apple's from its
// Marketing Tools toolbox, Google's from the Play badge endpoint. Neither
// may be redrawn, recoloured, cropped or reproportioned, per both
// companies' brand guidelines, which is why they are committed as files
// rather than rebuilt as inline SVG.
const APPLE_BADGE_SRC = "/badges/app-store.svg";
const PLAY_BADGE_SRC = "/badges/google-play.png";

// Aspect ratio of Apple's official SVG (119.66407 × 40). The artwork has
// NO built-in clear space, so we add it ourselves below.
const APPLE_ASPECT = 119.66407 / 40;

// Google's official badge is a 646 × 250 canvas whose *ink* occupies only
// 564 × 168 — 41px of built-in clear space on all four sides. Measured off
// the asset's alpha channel, not taken from the docs.
const PLAY_ASPECT = 646 / 250;
const PLAY_INK_FRACTION = 168 / 250; // 0.672

// Apple asks for clear space around its badge; Google's is already baked
// into the canvas. Expressed as a fraction of visible artwork height.
const APPLE_CLEAR_SPACE_FRACTION = 0.1;

export interface AppStoreBadgesProps {
  /** Override the URLs. Defaults to {@link MOBILE_ADMIN_APP_LINKS}. */
  links?: AppStoreLinks;
  /**
   * Height in px of the *visible artwork*, matched across both badges —
   * not the element height, which differs because Google's canvas carries
   * built-in padding and Apple's does not. Sizing both elements to the
   * same height would render Google's artwork ~33% smaller and break the
   * equal-visual-weight rule both guidelines impose.
   */
  height?: number;
  className?: string;
}

/**
 * Renders a badge per configured platform, or nothing at all when neither
 * URL is set.
 */
export function AppStoreBadges({
  links = MOBILE_ADMIN_APP_LINKS,
  height = 40,
  className,
}: AppStoreBadgesProps) {
  const iosUrl = links.ios.trim();
  const androidUrl = links.android.trim();

  if (!iosUrl && !androidUrl) return null;

  // Equalise the *ink*: Apple's element is the artwork height; Google's is
  // scaled up so its inked area lands at the same height.
  const appleHeight = height;
  const appleWidth = height * APPLE_ASPECT;
  const appleMargin = height * APPLE_CLEAR_SPACE_FRACTION;

  const playHeight = height / PLAY_INK_FRACTION;
  const playWidth = playHeight * PLAY_ASPECT;

  return (
    <ul
      className={`flex flex-wrap items-center gap-x-4 gap-y-3 ${className ?? ""}`.trim()}
    >
      {iosUrl ? (
        <li>
          <a
            href={iosUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-block rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {/* eslint-disable-next-line @next/next/no-img-element -- @repo/ui
                stays framework-agnostic; explicit width/height prevents CLS. */}
            <img
              src={APPLE_BADGE_SRC}
              alt="Download Mark8ly Admin on the App Store"
              width={Math.round(appleWidth)}
              height={Math.round(appleHeight)}
              style={{ margin: `${appleMargin}px` }}
            />
          </a>
        </li>
      ) : null}

      {androidUrl ? (
        <li>
          <a
            href={androidUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-block rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            {/* eslint-disable-next-line @next/next/no-img-element -- see above */}
            <img
              src={PLAY_BADGE_SRC}
              alt="Get Mark8ly Admin on Google Play"
              width={Math.round(playWidth)}
              height={Math.round(playHeight)}
            />
          </a>
        </li>
      ) : null}
    </ul>
  );
}
