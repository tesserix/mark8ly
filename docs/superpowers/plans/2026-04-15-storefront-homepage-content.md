# Storefront Homepage Content — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let merchants author homepage hero + content sections from the admin, rendered per-theme across all 5 layouts without losing content when switching themes.

**Architecture:** Single JSONB column `homepage_content` on `store_branding` holding `{hero, sections[]}`. Backend validates shape. Admin adds a "Homepage" tab to `BrandingSettingsClient` with hero editor + sections editor (4 block types: text, image, featured_products, quote). Each storefront layout component consumes the same `content` prop and styles it in its own way.

**Tech Stack:** Go 1.26 + Gin + GORM (marketplace-api), Next.js 16 + React 19 + Tailwind (admin, storefront). Reuses `react-markdown` (installed by Pages CMS plan), existing collection API for `featured_products`.

**Depends on:** Pages CMS plan (`2026-04-15-storefront-pages-cms.md`) complete. Shares the JSONB-on-branding pattern, page-picker dropdown (for CTA URL), and `Markdown` helper.

## Spec reference

`docs/superpowers/specs/2026-04-15-storefront-homepage-content-design.md`.

---

## File structure

### marketplace-api (new)
- `services/marketplace-api/migrations/000031_branding_homepage_content.up.sql`
- `services/marketplace-api/migrations/000031_branding_homepage_content.down.sql`
- `services/marketplace-api/internal/branding/homepage_content.go` — types + validator
- `services/marketplace-api/internal/branding/homepage_content_test.go`

### marketplace-api (modify)
- `services/marketplace-api/migrations.go` — register migration 31
- `services/marketplace-api/internal/branding/models.go` — add `HomepageContent`
- `services/marketplace-api/internal/branding/service.go` — accept `HomepageContent` on update
- `services/marketplace-api/internal/branding/handler.go` — pass through

### apps/storefront (new)
- `apps/storefront/components/homepage/HeroSection.tsx` — shared hero renderer
- `apps/storefront/components/homepage/sections/TextSection.tsx`
- `apps/storefront/components/homepage/sections/ImageSection.tsx`
- `apps/storefront/components/homepage/sections/QuoteSection.tsx`
- `apps/storefront/components/homepage/sections/FeaturedProductsSection.tsx`
- `apps/storefront/components/homepage/SectionsRenderer.tsx` — dispatcher
- `apps/storefront/lib/api/marketplace-api.ts` — `HomepageContent` type

### apps/storefront (modify)
- `apps/storefront/app/page.tsx` — pull `homepage_content` from branding, pass to layout renderer
- `apps/storefront/components/layouts/EditorialLayout.tsx` — consume `content` prop
- `apps/storefront/components/layouts/MinimalLayout.tsx` — same
- `apps/storefront/components/layouts/ClassicShopLayout.tsx` — same
- `apps/storefront/components/layouts/CompactLayout.tsx` — same
- `apps/storefront/components/layouts/HeroFocusLayout.tsx` — same
- `apps/storefront/components/layouts/index.tsx` — update `LayoutProps` type

### apps/admin (new)
- `apps/admin/components/settings/HomepageTab.tsx` — tab body
- `apps/admin/components/settings/HeroEditor.tsx`
- `apps/admin/components/settings/HomepageSectionsEditor.tsx`
- `apps/admin/components/settings/sections/TextSectionForm.tsx`
- `apps/admin/components/settings/sections/ImageSectionForm.tsx`
- `apps/admin/components/settings/sections/QuoteSectionForm.tsx`
- `apps/admin/components/settings/sections/FeaturedProductsSectionForm.tsx`

### apps/admin (modify)
- `apps/admin/lib/api/marketplace-api.ts` — add `HomepageContent` + section types
- `apps/admin/components/settings/BrandingSettingsClient.tsx` — add Homepage tab, wire editor state into PATCH

---

## Phase A — Schema + passthrough backend

### Task 1: homepage_content migration

**Files:**
- Create: `services/marketplace-api/migrations/000031_branding_homepage_content.up.sql`
- Create: `services/marketplace-api/migrations/000031_branding_homepage_content.down.sql`
- Modify: `services/marketplace-api/migrations.go`

- [ ] **Step 1: Write up.sql**

```sql
ALTER TABLE store_branding
  ADD COLUMN IF NOT EXISTS homepage_content JSONB NOT NULL DEFAULT '{}'::jsonb;
```

- [ ] **Step 2: Write down.sql**

```sql
ALTER TABLE store_branding DROP COLUMN IF EXISTS homepage_content;
```

- [ ] **Step 3: Register + run** — expect `migrated to 31`.

- [ ] **Step 4: Commit** — `feat(marketplace-api): store_branding.homepage_content column`

---

### Task 2: HomepageContent types + validator

**Files:**
- Create: `services/marketplace-api/internal/branding/homepage_content.go`
- Create: `services/marketplace-api/internal/branding/homepage_content_test.go`

- [ ] **Step 1: Write failing tests**

```go
package branding

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHomepageContent_EmptyObject_OK(t *testing.T) {
	err := ValidateHomepageContent(json.RawMessage(`{}`))
	require.NoError(t, err)
}

func TestValidateHomepageContent_HeroDisabled_OK(t *testing.T) {
	err := ValidateHomepageContent(json.RawMessage(`{"hero":{"enabled":false}}`))
	require.NoError(t, err)
}

func TestValidateHomepageContent_HeroWithFields_OK(t *testing.T) {
	body := `{"hero":{"enabled":true,"heading":"Acme","subheading":"Hand","image_url":"https://x/y.jpg","cta_label":"Shop","cta_url":"/a"}}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.NoError(t, err)
}

func TestValidateHomepageContent_HeroHeadingTooLong_Errors(t *testing.T) {
	long := "{"
	for i := 0; i < 201; i++ {
		long += "x"
	}
	body := `{"hero":{"enabled":true,"heading":"` + long + `"}}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
}

func TestValidateHomepageContent_SectionText_OK(t *testing.T) {
	body := `{"sections":[{"type":"text","markdown":"## Hi"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.NoError(t, err)
}

func TestValidateHomepageContent_SectionUnknownType_Errors(t *testing.T) {
	body := `{"sections":[{"type":"bogus"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
}

func TestValidateHomepageContent_TooManySections_Errors(t *testing.T) {
	s := `{"type":"text","markdown":"x"}`
	body := `{"sections":[`
	for i := 0; i < 13; i++ {
		if i > 0 {
			body += ","
		}
		body += s
	}
	body += `]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
}
```

- [ ] **Step 2: Run** — expect fail.

- [ ] **Step 3: Implement**

```go
package branding

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxSections        = 12
	maxHeroHeading     = 200
	maxHeroSubheading  = 400
	maxCtaLabel        = 60
	maxTextMarkdown    = 20_000
	maxImageAlt        = 200
	maxImageCaption    = 200
	maxQuoteText       = 500
	maxQuoteAttrib     = 200
	maxSectionHeading  = 200
	maxFeaturedLimit   = 24
)

// ValidateHomepageContent checks the shape of a homepage_content JSONB
// blob. Returns a user-facing validation error or nil.
func ValidateHomepageContent(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var c struct {
		Hero     *heroInput     `json:"hero"`
		Sections []sectionInput `json:"sections"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("homepage_content: invalid JSON: %w", err)
	}
	if c.Hero != nil {
		if err := validateHero(c.Hero); err != nil {
			return err
		}
	}
	if len(c.Sections) > maxSections {
		return fmt.Errorf("homepage_content: at most %d sections allowed", maxSections)
	}
	for i, s := range c.Sections {
		if err := validateSection(s); err != nil {
			return fmt.Errorf("sections[%d]: %w", i, err)
		}
	}
	return nil
}

type heroInput struct {
	Enabled    *bool   `json:"enabled"`
	ImageURL   *string `json:"image_url"`
	Heading    *string `json:"heading"`
	Subheading *string `json:"subheading"`
	CtaLabel   *string `json:"cta_label"`
	CtaURL     *string `json:"cta_url"`
}

type sectionInput struct {
	Type            string `json:"type"`
	Heading         *string `json:"heading,omitempty"`
	Markdown        *string `json:"markdown,omitempty"`
	URL             *string `json:"url,omitempty"`
	Alt             *string `json:"alt,omitempty"`
	Caption         *string `json:"caption,omitempty"`
	CollectionSlug  *string `json:"collection_slug,omitempty"`
	Limit           *int    `json:"limit,omitempty"`
	Text            *string `json:"text,omitempty"`
	Attribution     *string `json:"attribution,omitempty"`
}

func validateHero(h *heroInput) error {
	if h.Heading != nil && len(*h.Heading) > maxHeroHeading {
		return fmt.Errorf("hero.heading: max %d chars", maxHeroHeading)
	}
	if h.Subheading != nil && len(*h.Subheading) > maxHeroSubheading {
		return fmt.Errorf("hero.subheading: max %d chars", maxHeroSubheading)
	}
	if h.CtaLabel != nil && len(*h.CtaLabel) > maxCtaLabel {
		return fmt.Errorf("hero.cta_label: max %d chars", maxCtaLabel)
	}
	return nil
}

func validateSection(s sectionInput) error {
	switch strings.ToLower(s.Type) {
	case "text":
		if s.Markdown == nil {
			return fmt.Errorf("text: markdown required")
		}
		if len(*s.Markdown) > maxTextMarkdown {
			return fmt.Errorf("text.markdown: max %d chars", maxTextMarkdown)
		}
	case "image":
		if s.URL == nil || *s.URL == "" {
			return fmt.Errorf("image: url required")
		}
		if s.Alt != nil && len(*s.Alt) > maxImageAlt {
			return fmt.Errorf("image.alt: max %d chars", maxImageAlt)
		}
		if s.Caption != nil && len(*s.Caption) > maxImageCaption {
			return fmt.Errorf("image.caption: max %d chars", maxImageCaption)
		}
	case "featured_products":
		if s.CollectionSlug == nil || *s.CollectionSlug == "" {
			return fmt.Errorf("featured_products: collection_slug required")
		}
		if s.Limit != nil && (*s.Limit < 1 || *s.Limit > maxFeaturedLimit) {
			return fmt.Errorf("featured_products.limit: 1..%d", maxFeaturedLimit)
		}
	case "quote":
		if s.Text == nil || *s.Text == "" {
			return fmt.Errorf("quote: text required")
		}
		if len(*s.Text) > maxQuoteText {
			return fmt.Errorf("quote.text: max %d chars", maxQuoteText)
		}
		if s.Attribution != nil && len(*s.Attribution) > maxQuoteAttrib {
			return fmt.Errorf("quote.attribution: max %d chars", maxQuoteAttrib)
		}
	default:
		return fmt.Errorf("unknown section type %q", s.Type)
	}
	return nil
}
```

- [ ] **Step 4: Run** — expect pass.

- [ ] **Step 5: Commit** — `feat(marketplace-api): HomepageContent validator`

---

### Task 3: Branding model + service + handler accept homepage_content

**Files:**
- Modify: `services/marketplace-api/internal/branding/models.go`
- Modify: `services/marketplace-api/internal/branding/service.go`
- Modify: `services/marketplace-api/internal/branding/handler.go`

- [ ] **Step 1: Add field to StoreBranding**

```go
HomepageContent datatypes.JSON `gorm:"column:homepage_content;type:jsonb;not null;default:'{}'"` json:"homepage_content"`
```

(Import `gorm.io/datatypes` if not already.)

- [ ] **Step 2: Add to update input + merge in `service.go`**

Follow the existing pattern used for `FooterSections` (landing in the sibling Pages CMS plan). If that landed before this plan, the import + patch shape will already be established.

- [ ] **Step 3: Handler accepts `homepage_content` in PATCH body**

Call `ValidateHomepageContent(req.HomepageContent)` before writing; surface errors as `apperrors.ValidationFailed`.

- [ ] **Step 4: Integration test** — PATCH with a valid content body round-trips; PATCH with an unknown section type returns 400.

- [ ] **Step 5: Commit** — `feat(marketplace-api): branding accepts homepage_content`

---

## Phase B — Storefront default rendering

### Task 4: Shared section renderers + types

**Files:**
- Create: `apps/storefront/components/homepage/SectionsRenderer.tsx`
- Create: `apps/storefront/components/homepage/sections/TextSection.tsx`
- Create: `apps/storefront/components/homepage/sections/ImageSection.tsx`
- Create: `apps/storefront/components/homepage/sections/QuoteSection.tsx`
- Create: `apps/storefront/components/homepage/sections/FeaturedProductsSection.tsx`
- Modify: `apps/storefront/lib/api/marketplace-api.ts` — add types

- [ ] **Step 1: Define types**

```ts
// apps/storefront/lib/api/marketplace-api.ts
export interface HomepageHero {
  enabled: boolean;
  image_url?: string | null;
  heading?: string | null;
  subheading?: string | null;
  cta_label?: string | null;
  cta_url?: string | null;
}

export type HomepageSection =
  | { type: "text"; heading?: string | null; markdown: string }
  | { type: "image"; url: string; alt?: string | null; caption?: string | null; heading?: string | null }
  | { type: "featured_products"; collection_slug: string; limit?: number; heading?: string | null }
  | { type: "quote"; text: string; attribution?: string | null; heading?: string | null };

export interface HomepageContent {
  hero?: HomepageHero;
  sections?: HomepageSection[];
}
```

- [ ] **Step 2: Write each section component.**

Keep them thin and theme-neutral. They accept `section` + `theme` and
render with brand-appropriate default typography (Serif for headings
if theme.typography.headingFont is set, otherwise defaults). Theme-
specific positioning/grid is the *layout*'s job — these render the
content.

- [ ] **Step 3: SectionsRenderer dispatcher**

```tsx
// apps/storefront/components/homepage/SectionsRenderer.tsx
import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";
import { TextSection } from "./sections/TextSection";
import { ImageSection } from "./sections/ImageSection";
import { FeaturedProductsSection } from "./sections/FeaturedProductsSection";
import { QuoteSection } from "./sections/QuoteSection";

export function SectionsRenderer({ sections, theme }: { sections: HomepageSection[]; theme: StorefrontTheme }) {
  return (
    <div className="space-y-14">
      {sections.map((s, i) => {
        const key = `${s.type}-${i}`;
        switch (s.type) {
          case "text":              return <TextSection key={key} section={s} theme={theme} />;
          case "image":             return <ImageSection key={key} section={s} theme={theme} />;
          case "featured_products": return <FeaturedProductsSection key={key} section={s} theme={theme} />;
          case "quote":             return <QuoteSection key={key} section={s} theme={theme} />;
          default:                  return null; // unknown type — render nothing rather than throw
        }
      })}
    </div>
  );
}
```

- [ ] **Step 4: Commit** — `feat(storefront): homepage section renderers (text/image/quote/featured_products)`

---

### Task 5: Shared HeroSection component

**Files:**
- Create: `apps/storefront/components/homepage/HeroSection.tsx`

- [ ] **Step 1: Write it**

```tsx
// apps/storefront/components/homepage/HeroSection.tsx
import Link from "next/link";
import type { HomepageHero } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

export function HeroSection({ hero, theme, fallbackHeading }: {
  hero: HomepageHero | undefined | null;
  theme: StorefrontTheme;
  fallbackHeading: string;
}) {
  const enabled = hero?.enabled ?? true;
  if (!enabled) return null;
  const heading = hero?.heading?.trim() || fallbackHeading;
  const subheading = hero?.subheading?.trim() ?? "";
  const imageURL = hero?.image_url ?? null;
  const cta = hero?.cta_label && hero.cta_url ? { label: hero.cta_label, url: hero.cta_url } : null;

  return (
    <section className="relative isolate overflow-hidden">
      {imageURL && (
        <img
          src={imageURL}
          alt=""
          className="absolute inset-0 -z-10 h-full w-full object-cover opacity-70"
        />
      )}
      <div className="relative px-8 py-24">
        <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
          {heading}
        </h1>
        {subheading && <p className="mt-5 max-w-2xl text-lg text-foreground-secondary">{subheading}</p>}
        {cta && (
          <Link
            href={cta.url}
            className="mt-8 inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
          >
            {cta.label}
          </Link>
        )}
      </div>
    </section>
  );
}
```

Themes can wrap this with their own aspect ratio / position / treatment.

- [ ] **Step 2: Commit** — `feat(storefront): shared HeroSection`

---

### Task 6: Thread `content` through layout renderers

**Files:**
- Modify: `apps/storefront/components/layouts/index.tsx`
- Modify: `apps/storefront/components/layouts/{Editorial,Minimal,ClassicShop,Compact,HeroFocus}Layout.tsx`

- [ ] **Step 1: Extend `LayoutProps`**

```ts
interface LayoutProps {
  store: PublicStore;
  theme: StorefrontTheme;
  content: HomepageContent | null;
}
```

- [ ] **Step 2: For each layout file,**
  - Render `<HeroSection hero={content?.hero} theme={theme} fallbackHeading={store.name} />` at the top (each theme wraps/positions this differently).
  - Render `<SectionsRenderer sections={content?.sections ?? []} theme={theme} />` in the theme's content region.
  - Keep theme-specific decoration (editorial's asymmetric grid, compact's sidebar, hero-focus's full-viewport background).
  - If `content` is null or empty, the layouts still render their theme-specific placeholder content (existing behaviour — don't break it).

- [ ] **Step 3: Commit** — `feat(storefront): layouts consume homepage content prop`

---

### Task 7: Wire homepage_content into app/page.tsx

**Files:**
- Modify: `apps/storefront/app/page.tsx`

- [ ] **Step 1:** Read `branding.homepage_content` alongside the existing branding fetch. Pass into `StorefrontLayoutRenderer`.

- [ ] **Step 2: Smoke test** — a store with empty `homepage_content` renders today's placeholder; a store with `{hero:{enabled:true, heading:"Hello"}}` shows "Hello" as the hero title.

- [ ] **Step 3: Commit** — `feat(storefront): pass homepage_content into layout renderer`

---

## Phase C — Admin Homepage tab

### Task 8: Admin types + client helpers

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts`

- [ ] Mirror the storefront types (`HomepageHero`, `HomepageSection`, `HomepageContent`) so the admin form has full typing.
- [ ] No new endpoint — `homepage_content` rides on the existing branding PATCH.
- [ ] Commit — `feat(admin): homepage content types`

### Task 9: HeroEditor component

**Files:**
- Create: `apps/admin/components/settings/HeroEditor.tsx`

- [ ] Props: `{ value: HomepageHero; onChange: (next: HomepageHero) => void; pages: PageSummary[]; }`
- [ ] Toggle "Show hero on homepage"
- [ ] Image URL field (for v1; upload UX deferred)
- [ ] Heading + subheading inputs
- [ ] CTA label + CTA URL fields (URL field uses same page-picker + freeform pattern as FooterSectionsEditor)
- [ ] Commit — `feat(admin): HeroEditor`

### Task 10: HomepageSectionsEditor + per-type forms

**Files:**
- Create: `apps/admin/components/settings/HomepageSectionsEditor.tsx`
- Create: `apps/admin/components/settings/sections/TextSectionForm.tsx`
- Create: `apps/admin/components/settings/sections/ImageSectionForm.tsx`
- Create: `apps/admin/components/settings/sections/QuoteSectionForm.tsx`
- Create: `apps/admin/components/settings/sections/FeaturedProductsSectionForm.tsx`

- [ ] Dispatcher renders each section as a card with its type-specific form.
- [ ] "+ Add section" dropdown with the 4 block types.
- [ ] Up/down arrows per card to reorder.
- [ ] Remove button per card (confirm).
- [ ] Each per-type form is thin — inputs matching the validator's field set.
- [ ] For `featured_products`, fetch collections via existing admin API; render a select.
- [ ] For `text`, use a plain textarea + `react-markdown` preview panel (mirrors the Pages editor treatment from the Pages CMS plan).
- [ ] Commit — `feat(admin): homepage sections editor`

### Task 11: HomepageTab + wire into BrandingSettingsClient

**Files:**
- Create: `apps/admin/components/settings/HomepageTab.tsx`
- Modify: `apps/admin/components/settings/BrandingSettingsClient.tsx`

- [ ] Add `"homepage"` to the `Tab` union and the `TABS` array (between "layout" and "footer").
- [ ] Add `{tab === "homepage" && <HomepageTab form={form} patch={patch} editable={editable} />}` to the render switch.
- [ ] `HomepageTab` composes `HeroEditor` + `HomepageSectionsEditor` and `patch({ homepage_content: ... })` on changes.
- [ ] Ensure `UpdateBrandingInput` carries `homepage_content` through to the server action.
- [ ] Commit — `feat(admin): homepage tab in branding`

---

## Phase D — Polish

### Task 12: Markdown safety parity
- [ ] Admin text preview + storefront text renderer both use `react-markdown` with `skipHtml` (same as Pages CMS).
- [ ] Add an XSS unit test that confirms `<script>` tags are stripped in both preview and live render.

### Task 13: Collection deletion handling
- [ ] When `featured_products` references a missing collection, storefront renders an empty `<section>` with a small "collection unavailable" placeholder (only visible to admin via query param? or always silent? — keep it silent for customer view, admin surfaces a warning badge on the section card).

### Task 14: Default fallback when content is empty
- [ ] Verify: a store with `homepage_content = {}` still renders the existing theme-specific placeholder (no regression).

### Task 15: Deploy + verify
- [ ] CI + k8s PR + ArgoCD sync + rollout.
- [ ] On a real store, add a hero + 2 sections + 1 featured_products; verify on storefront; switch theme → same content re-styled.

---

## Done criteria

- [ ] `store_branding.homepage_content` exists in prod.
- [ ] Backend validator rejects malformed bodies with clear error codes.
- [ ] Admin "Homepage" tab lets merchants set hero + add/edit/reorder/remove 4 section types.
- [ ] All 5 storefront themes render the same content in their own style.
- [ ] Switching theme preserves content.
- [ ] A new store with no homepage_content still has a working homepage.
- [ ] XSS test: `<script>` in a text section renders as inert text.
- [ ] No regression on existing storefront tests.
