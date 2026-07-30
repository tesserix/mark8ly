import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { readFileSync } from 'node:fs'
import path from 'node:path'
import {
  AppStoreBadges,
  MOBILE_ADMIN_APP_LINKS,
  type AppStoreLinks,
} from './app-store-badges'

const BOTH: AppStoreLinks = {
  ios: 'https://apps.apple.com/app/apple-store/id1234567890',
  android: 'https://play.google.com/store/apps/details?id=com.mark8ly.admin',
}

describe('AppStoreBadges — per-platform gating', () => {
  // The whole point of the component: a platform whose URL is not
  // configured must not render, so the App Store badge cannot ship as a
  // dead link before the app is approved.
  // 🔴 These assert on the IMG (via alt text), not on role="link". An
  // <a href=""> loses its implicit link role, so a role-based query would
  // report the badge as absent even when the gate is broken and the anchor
  // IS rendered — i.e. it would pass for the wrong reason. Verified: with
  // the gate removed, the alt-text assertions go red and role-based ones
  // did not.
  it('renders both badges when both URLs are configured', () => {
    render(<AppStoreBadges links={BOTH} />)
    expect(screen.getByAltText(/App Store/i)).toBeInTheDocument()
    expect(screen.getByAltText(/Google Play/i)).toBeInTheDocument()
  })

  it('renders ONLY Google Play when the iOS URL is empty', () => {
    render(<AppStoreBadges links={{ ...BOTH, ios: '' }} />)
    expect(screen.queryByAltText(/App Store/i)).toBeNull()
    expect(screen.getByAltText(/Google Play/i)).toBeInTheDocument()
  })

  it('renders ONLY the App Store when the Android URL is empty', () => {
    render(<AppStoreBadges links={{ ...BOTH, android: '' }} />)
    expect(screen.getByAltText(/App Store/i)).toBeInTheDocument()
    expect(screen.queryByAltText(/Google Play/i)).toBeNull()
  })

  it('renders nothing at all when neither URL is configured', () => {
    const { container } = render(<AppStoreBadges links={{ ios: '', android: '' }} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('treats a whitespace-only URL as unconfigured', () => {
    render(<AppStoreBadges links={{ ...BOTH, ios: '   ' }} />)
    expect(screen.queryByAltText(/App Store/i)).toBeNull()
  })

  it('never renders an anchor with an empty href', () => {
    render(<AppStoreBadges links={{ ...BOTH, ios: '' }} />)
    for (const a of Array.from(document.querySelectorAll('a'))) {
      expect(a.getAttribute('href')).toBeTruthy()
    }
  })

  it('points each badge at its own configured URL, opened safely', () => {
    render(<AppStoreBadges links={BOTH} />)
    const ios = screen.getByRole('link', { name: /App Store/i })
    const android = screen.getByRole('link', { name: /Google Play/i })
    expect(ios).toHaveAttribute('href', BOTH.ios)
    expect(android).toHaveAttribute('href', BOTH.android)
    for (const el of [ios, android]) {
      expect(el).toHaveAttribute('target', '_blank')
      expect(el).toHaveAttribute('rel', expect.stringContaining('noopener') as unknown as string)
    }
  })
})

describe('AppStoreBadges — shipped configuration', () => {
  // Guards the deliberate hold on iOS. When the app is approved this test
  // is the one to update, which makes the change explicit rather than
  // something that drifts in unnoticed.
  it('ships with Play live and iOS deliberately withheld', () => {
    expect(MOBILE_ADMIN_APP_LINKS.android).toBe(
      'https://play.google.com/store/apps/details?id=com.mark8ly.admin',
    )
    expect(MOBILE_ADMIN_APP_LINKS.ios).toBe('')
  })

  it('renders only the Play badge with the shipped defaults', () => {
    render(<AppStoreBadges />)
    expect(screen.getByAltText(/Google Play/i)).toBeInTheDocument()
    expect(screen.queryByAltText(/App Store/i)).toBeNull()
  })
})

describe('AppStoreBadges — equal visual weight', () => {
  // Google's canvas carries 41px of built-in clear space per side (ink is
  // 168 of 250 = 67.2% of canvas height); Apple's artwork has none. Sizing
  // both elements to the same height would render Google's ~33% smaller,
  // which breaches the equal-prominence rule in both brand guidelines.
  // This test exists to stop someone "simplifying" that to one height.
  it('scales the Play element taller so the inked areas match', () => {
    render(<AppStoreBadges links={BOTH} height={40} />)
    const apple = screen.getByAltText(/App Store/i)
    const play = screen.getByAltText(/Google Play/i)

    const appleH = Number(apple.getAttribute('height'))
    const playH = Number(play.getAttribute('height'))

    expect(appleH).toBe(40)
    expect(playH).toBe(60) // 40 / 0.672, rounded
    // Ink heights must agree within a rounding pixel.
    expect(Math.abs(playH * (168 / 250) - appleH)).toBeLessThanOrEqual(1)
  })

  it('preserves each badge’s official aspect ratio', () => {
    render(<AppStoreBadges links={BOTH} height={40} />)
    const apple = screen.getByAltText(/App Store/i)
    const play = screen.getByAltText(/Google Play/i)

    const ratio = (el: HTMLElement) =>
      Number(el.getAttribute('width')) / Number(el.getAttribute('height'))

    expect(ratio(apple)).toBeCloseTo(119.66407 / 40, 1)
    expect(ratio(play)).toBeCloseTo(646 / 250, 1)
  })
})

describe('AppStoreBadges — accessibility', () => {
  it('gives each badge specific alt text, not a generic label', () => {
    render(<AppStoreBadges links={BOTH} />)
    // Naming the app matters: "App Store badge" tells a screen-reader user
    // nothing about what they are downloading.
    expect(screen.getByAltText('Download Mark8ly Admin on the App Store')).toBeInTheDocument()
    expect(screen.getByAltText('Get Mark8ly Admin on Google Play')).toBeInTheDocument()
  })

  it('exposes the badges as a list of links', () => {
    render(<AppStoreBadges links={BOTH} />)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
  })
})

describe('badge artwork is present and unmodified in every consuming app', () => {
  // The component hardcodes /badges/*, so a missing asset is a broken
  // image in production that no type or render test would catch.
  const REPO = path.resolve(__dirname, '../../..')
  const APPS = ['onboarding', 'admin']
  const ASSETS = ['app-store.svg', 'google-play.png']

  it.each(APPS.flatMap((app) => ASSETS.map((asset) => [app, asset] as const)))(
    'apps/%s/public/badges/%s exists and is non-empty',
    (app, asset) => {
      const buf = readFileSync(path.join(REPO, 'apps', app, 'public', 'badges', asset))
      expect(buf.byteLength).toBeGreaterThan(1000)
    },
  )

  it('serves byte-identical artwork from both apps', () => {
    for (const asset of ASSETS) {
      const [a, b] = APPS.map((app) =>
        readFileSync(path.join(REPO, 'apps', app, 'public', 'badges', asset)).toString('base64'),
      )
      expect(a).toBe(b)
    }
  })

  it('keeps the official Play canvas at 646x250, so the ink ratio stays valid', () => {
    const png = readFileSync(path.join(REPO, 'apps/admin/public/badges/google-play.png'))
    // PNG IHDR: width/height are big-endian uint32 at byte offsets 16 and 20.
    expect(png.readUInt32BE(16)).toBe(646)
    expect(png.readUInt32BE(20)).toBe(250)
  })
})
