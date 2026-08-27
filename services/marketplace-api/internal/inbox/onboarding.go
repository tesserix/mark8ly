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
// Upstream's ListSessions used to order created_at DESC with no way to ask
// for anything else, which put the genuinely-stalled sessions — the least
// recently active — deepest in the result set, exactly where a first-page
// request could not see them. #406 added an ordering parameter; fetchStalled
// now requests `last_activity_at_asc`, so the oldest rows arrive in the
// window rather than behind it.
//
// Two things still shape what this provider can promise:
//
// Every filter this provider applies now reaches upstream — abandoned, the
// idle-hours threshold and the tenant — so the remote's own total is this
// queue's true size. Count therefore requests a single row rather than paging
// MaxAggregateItems to count what came back, which used to under-report for
// any queue larger than that bound.
//
// List's client-side sort by WaitingSince and its idle/tenant re-checks still
// run, but they are belt-and-braces rather than load-bearing: a client-side
// pass reorders and discards rows it was given, it cannot change which rows it
// was given. That distinction is the whole of #406.
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
// sessionsParams builds the upstream request. Every filter this provider
// applies is expressed HERE, not on the response: upstream runs the same
// filter over its count query and its page query, so a filter left on the
// client makes pagination.total count rows the provider then discards, and
// makes rows beyond the requested window unreachable (#406).
func (p *OnboardingProvider) sessionsParams(f Filter, page, limit int) onboardingfunnel.SessionsParams {
	abandoned := true
	idleMin := p.idleThresholdHours
	return onboardingfunnel.SessionsParams{
		Page:         page,
		Limit:        limit,
		Status:       f.Status,
		Abandoned:    &abandoned,
		TenantID:     f.TenantID,
		IdleHoursMin: &idleMin,
		// Ask upstream for least-recently-active first (#406). Without this
		// the remote orders created_at DESC, so page 1 returns the NEWEST
		// sessions and the genuinely stalled ones — the rows this queue
		// exists to surface — sit deepest in the result set, off the window
		// entirely. List's client-side sort cannot recover them: it reorders
		// what it was given, it does not change what it was given.
		Order: "last_activity_at_asc",
	}
}

func (p *OnboardingProvider) fetchStalled(ctx context.Context, f Filter, limit int) ([]onboardingfunnel.Session, error) {
	page := f.Page
	if page <= 0 {
		page = 1
	}
	res, err := p.client.ListSessions(ctx, p.sessionsParams(f, page, limit))
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	// Both filters now run upstream (see sessionsParams). These re-checks are
	// kept as a cheap assertion that the remote honoured them: if a future
	// upstream silently ignores a parameter, the queue shows fewer rows rather
	// than wrong ones, and Count -- which trusts upstream's total -- is the
	// thing that would then visibly disagree with the list.
	sessions := make([]onboardingfunnel.Session, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		if s.IdleHours < p.idleThresholdHours {
			continue
		}
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
// Count reports upstream's filtered total.
//
// It used to page MaxAggregateItems rows and count what came back, which
// under-reported for any queue larger than that bound and spent a 500-row
// fetch to answer a number upstream already had. Now that the abandoned,
// idle-threshold and tenant filters all reach the remote (see sessionsParams),
// its total IS this queue's size, so one row is requested instead of 500 (#406).
func (p *OnboardingProvider) Count(ctx context.Context, f Filter) (int64, error) {
	res, err := p.client.ListSessions(ctx, p.sessionsParams(f, 1, 1))
	if err != nil {
		return 0, err
	}
	if res == nil {
		return 0, nil
	}
	return res.Total, nil
}
