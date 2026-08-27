package inbox

import (
	"context"
	"sort"

	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// SessionLister is the slice of the onboarding funnel client this provider
// needs. Declaring it here rather than importing the concrete client keeps the
// provider unit-testable with a fake and documents the exact dependency.
type SessionLister interface {
	ListSessions(ctx context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error)
}

// OnboardingProvider surfaces abandoned onboarding sessions.
//
// Upstream's ListSessions has no ordering parameter (it orders created_at
// DESC, hardcoded) and no way to request "oldest abandoned first" directly.
// Narrowing to Abandoned=true (see List) shrinks the population this provider
// asks for enormously, making it far more likely the requested window
// contains the genuinely-longest-waiting sessions, and this provider sorts
// what it gets by WaitingSince ascending before returning it. But if abandoned
// sessions ever exceed the page size requested from upstream, the oldest
// ones can still be unreachable — this is a real, currently-unclosed gap in
// the underlying API, not something this provider can paper over.
type OnboardingProvider struct {
	client             SessionLister
	idleThresholdHours float64
}

func NewOnboardingProvider(c SessionLister, idleThresholdHours float64) *OnboardingProvider {
	if idleThresholdHours <= 0 {
		idleThresholdHours = 48
	}
	return &OnboardingProvider{client: c, idleThresholdHours: idleThresholdHours}
}

func (p *OnboardingProvider) Kind() string { return KindOnboardingStalled }

// fetchStalled requests up to limit sessions from upstream, narrowed to
// abandoned sessions, and applies the same idle-threshold and tenant
// filtering used by both List and Count so the two cannot drift.
func (p *OnboardingProvider) fetchStalled(ctx context.Context, f Filter, limit int) ([]onboardingfunnel.Session, error) {
	page := f.Page
	if page <= 0 {
		page = 1
	}
	abandoned := true
	res, err := p.client.ListSessions(ctx, onboardingfunnel.SessionsParams{
		Page:      page,
		Limit:     limit,
		Status:    f.Status,
		Abandoned: &abandoned,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	sessions := make([]onboardingfunnel.Session, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		if s.IdleHours < p.idleThresholdHours {
			continue
		}
		// The remote SessionsParams has no tenant field, so the request cannot
		// filter by tenant — this match happens client-side on the response.
		// Do not "simplify" this into a SessionsParams field; it does not exist.
		// A session with no TenantID has not been linked to a tenant yet, so it
		// is not this tenant's when a specific tenant is requested.
		if f.TenantID != "" && (s.TenantID == nil || *s.TenantID != f.TenantID) {
			continue
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (p *OnboardingProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	sessions, err := p.fetchStalled(ctx, f, limit)
	if err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, Item{
			ID:           s.ID,
			Kind:         KindOnboardingStalled,
			Title:        s.Email,
			Subtitle:     "Onboarding idle",
			WaitingSince: s.LastActivityAt,
			Severity:     SeverityNormal,
			Href:         "/admin/onboarding/sessions/" + s.ID,
			Actions:      []Action{{ID: "nudge", Label: "Send reminder", Destructive: false}},
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].WaitingSince.Before(items[j].WaitingSince)
	})
	return items, nil
}

// Count answers how many abandoned, idle-enough sessions are waiting,
// independent of the incoming Filter's Page/Limit.
//
// It saturates at MaxAggregateItems: platform-api's
// /internal/onboarding/sessions has no filtered-count-only endpoint, so this
// counts by fetching up to MaxAggregateItems sessions and filtering
// client-side the same way List does. A total that changes as an operator
// pages forward is worse than a total that's honestly bounded.
func (p *OnboardingProvider) Count(ctx context.Context, f Filter) (int64, error) {
	countFilter := f
	countFilter.Page = 1
	sessions, err := p.fetchStalled(ctx, countFilter, MaxAggregateItems)
	if err != nil {
		return 0, err
	}
	return int64(len(sessions)), nil
}
