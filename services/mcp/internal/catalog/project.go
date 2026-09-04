package catalog

import (
	"sort"
)

// projectProduct transforms a storefront product mirror into the agent-facing
// Product type. Tax fields and internal ID are excluded by being absent from
// the result struct.
func projectProduct(in storefrontProduct) Product {
	p := Product{
		Handle:     in.Handle,
		Title:      in.Title,
		PriceMin:   in.PriceRange.Min,
		PriceMax:   in.PriceRange.Max,
		Currency:   in.PriceRange.CurrencyCode,
		Categories: extractCategorySlugs(in.Categories),
		ImageURLs:  extractImageURLs(in.Media),
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	return p
}

// extractCategorySlugs projects category refs to their slugs only.
// Returns an empty non-nil slice when there are no categories, never nil.
func extractCategorySlugs(refs []storefrontCategoryRef) []string {
	if len(refs) == 0 {
		return []string{}
	}
	slugs := make([]string, len(refs))
	for i, ref := range refs {
		slugs[i] = ref.Slug
	}
	return slugs
}

// extractImageURLs filters media by type "image", sorts by position, and
// extracts URLs. Returns an empty non-nil slice when there are no images, never nil.
func extractImageURLs(media []storefrontMedia) []string {
	if len(media) == 0 {
		return []string{}
	}
	// Filter images only
	var images []storefrontMedia
	for _, m := range media {
		if m.MediaType == "image" {
			images = append(images, m)
		}
	}
	if len(images) == 0 {
		return []string{}
	}
	// Stable: upstream can give two images the same position, and an unstable
	// sort would order them arbitrarily — the same product could come back
	// with its images in a different order on identical input.
	sort.SliceStable(images, func(i, j int) bool {
		return images[i].Position < images[j].Position
	})
	// Extract URLs
	urls := make([]string, len(images))
	for i, img := range images {
		urls[i] = img.URL
	}
	return urls
}

// projectCategory transforms a storefront category mirror into the agent-facing
// Category type.
func projectCategory(in storefrontCategory) Category {
	return Category{
		Name:     in.Name,
		Slug:     in.Slug,
		Featured: in.Featured,
	}
}

// projectBranding transforms a storefront branding mirror into the agent-facing
// Branding type. The ColorAccent field becomes AccentColor. Pointer types are
// flattened to their zero values, and the announcement text is carried through
// only when AnnouncementActive is set.
func projectBranding(in storefrontBranding) Branding {
	logoURL := ""
	if in.LogoURL != nil {
		logoURL = *in.LogoURL
	}
	tagline := ""
	if in.Tagline != nil {
		tagline = *in.Tagline
	}
	// The announcement is projected only when the merchant has it switched
	// ON. Text left behind a disabled switch is stale copy the storefront
	// itself does not render (PromotionBar.tsx requires both), and reporting
	// it to an agent is a wrong answer that reads like a right one.
	announcement := ""
	if in.AnnouncementActive && in.AnnouncementText != nil {
		announcement = *in.AnnouncementText
	}
	return Branding{
		LogoURL:      logoURL,
		Tagline:      tagline,
		AccentColor:  in.ColorAccent,
		Announcement: announcement,
		Promotions:   projectPromotions(in.Promotions),
	}
}

// projectPromotions transforms storefront promotions into agent-facing Promotions.
// Returns an empty non-nil slice when there are no promotions, never nil.
func projectPromotions(in []storefrontPromotion) []Promotion {
	if len(in) == 0 {
		return []Promotion{}
	}
	out := make([]Promotion, len(in))
	for i, p := range in {
		out[i] = projectPromotion(p)
	}
	return out
}

// projectPromotion transforms a single storefront promotion into an agent-facing
// Promotion. Pointer types are flattened to their zero values.
func projectPromotion(in storefrontPromotion) Promotion {
	code := ""
	if in.CouponCode != nil {
		code = *in.CouponCode
	}
	return Promotion{
		Label:      in.Label,
		CouponCode: code,
	}
}
