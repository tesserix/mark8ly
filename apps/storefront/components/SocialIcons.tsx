/**
 * Monoline brand icons for the storefront footer.
 * Kept small and self-contained — the storefront's lucide-react is too
 * old for brand glyphs, and inlining SVGs lets us match the mark8ly
 * editorial stroke without extra deps. Icons render at whatever size
 * the surrounding element dictates via currentColor.
 */

import type { SVGProps } from "react";

type IconProps = SVGProps<SVGSVGElement>;

const baseProps: IconProps = {
  width: 20,
  height: 20,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.5,
  strokeLinecap: "round",
  strokeLinejoin: "round",
  "aria-hidden": true,
};

export function InstagramIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <rect x="3" y="3" width="18" height="18" rx="5" />
      <circle cx="12" cy="12" r="4" />
      <circle cx="17.5" cy="6.5" r="0.6" fill="currentColor" stroke="none" />
    </svg>
  );
}

export function XIcon(props: IconProps) {
  // Stylised X / Twitter monogram — two crossed diagonals inside a square area.
  return (
    <svg {...baseProps} {...props}>
      <path d="M4 4 L20 20" />
      <path d="M20 4 L4 20" />
    </svg>
  );
}

export function FacebookIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M18 2h-3a5 5 0 0 0-5 5v3H7v4h3v8h4v-8h3l1-4h-4V7a1 1 0 0 1 1-1h3z" />
    </svg>
  );
}

export function TikTokIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <path d="M9 12a4 4 0 1 0 4 4V4c0 3 2 5 5 5" />
    </svg>
  );
}

export function YouTubeIcon(props: IconProps) {
  return (
    <svg {...baseProps} {...props}>
      <rect x="2.5" y="6" width="19" height="12" rx="3" />
      <path d="M10 9.5v5l4.5-2.5z" fill="currentColor" stroke="none" />
    </svg>
  );
}
