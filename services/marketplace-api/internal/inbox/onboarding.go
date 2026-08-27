package inbox

import (
	"context"

	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// SessionLister is the slice of the onboarding funnel client this provider
// needs. Declaring it here rather than importing the concrete client keeps the
// provider unit-testable with a fake and documents the exact dependency.
type SessionLister interface {
	ListSessions(ctx context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error)
}

// OnboardingProvider surfaces onboarding sessions idle beyond a threshold.
//
// It is the only remote provider: the data lives in platform-api and is reached
// through the same HTTP client that already serves /admin/onboarding/sessions.
// Errors are returned, never swallowed — the aggregator marks this kind
// degraded so an operator can tell "none" from "we could not ask".
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

func (p *OnboardingProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	res, err := p.client.ListSessions(ctx, onboardingfunnel.SessionsParams{Page: 1, Limit: limit})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	items := make([]Item, 0, len(res.Sessions))
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
	return items, nil
}

func (p *OnboardingProvider) Count(ctx context.Context, f Filter) (int64, error) {
	items, err := p.List(ctx, f)
	if err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}
