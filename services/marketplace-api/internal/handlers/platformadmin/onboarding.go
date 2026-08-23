package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// OnboardingFunnel is the subset of onboardingfunnel.Client this handler
// needs. Declared here so the handler can be tested with a stub and so the
// transport package stays swappable.
type OnboardingFunnel interface {
	GetFunnel(ctx context.Context, p onboardingfunnel.FunnelParams) (*onboardingfunnel.FunnelStats, error)
	ListSessions(ctx context.Context, p onboardingfunnel.SessionsParams) (*onboardingfunnel.SessionsResult, error)
}

// OnboardingFunnelHandler serves the platform console's onboarding funnel
// and session list (#283). It owns the wire shape and nothing else — the
// counters, the shared abandoned predicate and the 24h cutoff all live in
// platform-api, which owns the data.
type OnboardingFunnelHandler struct {
	client OnboardingFunnel
	logger *slog.Logger
}

// NewOnboardingFunnelHandler constructs the handler. logger may be nil.
func NewOnboardingFunnelHandler(client OnboardingFunnel, logger *slog.Logger) *OnboardingFunnelHandler {
	return &OnboardingFunnelHandler{client: client, logger: logger}
}

// Register mounts both routes on the supplied group.
func (h *OnboardingFunnelHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/onboarding/funnel", h.funnel)
	g.GET("/admin/onboarding/sessions", h.sessions)
}

// funnelCountsRow is the top-level counters shape, shared by the funnel
// response's root and (narrowed, see last24hRow) its last_24h field.
type funnelCountsRow struct {
	Started       int64 `json:"started"`
	EmailVerified int64 `json:"email_verified"`
	Completed     int64 `json:"completed"`
	InFlight      int64 `json:"in_flight"`
	Abandoned     int64 `json:"abandoned"`
}

// last24hRow is deliberately narrower than funnelCountsRow — the contract
// pins only started/completed for the live pulse, not the full counter set.
type last24hRow struct {
	Started   int64 `json:"started"`
	Completed int64 `json:"completed"`
}

type funnelWindowRow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// funnelRow is the funnel wire shape. MedianCompletionSeconds has NO
// omitempty: the console must receive an explicit JSON null when nothing
// completed in the window, never a missing key and never 0 (which reads as
// "instant completion").
type funnelRow struct {
	funnelCountsRow
	MedianCompletionSeconds *float64        `json:"median_completion_seconds"`
	Last24h                 last24hRow      `json:"last_24h"`
	Window                  funnelWindowRow `json:"window"`
}

func (h *OnboardingFunnelHandler) funnel(c *gin.Context) {
	p := onboardingfunnel.FunnelParams{}
	if t, ok := parseTime(c.Query("created_from")); ok {
		p.CreatedFrom = t
	}
	if t, ok := parseTime(c.Query("created_to")); ok {
		p.CreatedTo = t
	}

	stats, err := h.client.GetFunnel(c.Request.Context(), p)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Single object, no pagination key — deliberately not reusing
	// listResponse/pagination here.
	c.JSON(http.StatusOK, gin.H{"data": toFunnelRow(stats)})
}

func toFunnelRow(s *onboardingfunnel.FunnelStats) funnelRow {
	return funnelRow{
		funnelCountsRow: funnelCountsRow{
			Started:       s.Started,
			EmailVerified: s.EmailVerified,
			Completed:     s.Completed,
			InFlight:      s.InFlight,
			Abandoned:     s.Abandoned,
		},
		MedianCompletionSeconds: s.MedianCompletionSeconds,
		Last24h: last24hRow{
			Started:   s.Last24h.Started,
			Completed: s.Last24h.Completed,
		},
		Window: funnelWindowRow{From: s.Window.From, To: s.Window.To},
	}
}

// sessionRow is the sessions wire shape. Projected field by field from
// onboardingfunnel.Session: the upstream row also carries `draft`, a JSONB
// blob of merchant-entered wizard data that must never reach the console.
// Naming every field here — rather than embedding or passing the client
// type through — is what makes that structurally impossible.
//
// CompletedAt and TenantID have NO omitempty: the contract pins them as
// explicit null until completion, not an omitted key.
type sessionRow struct {
	ID             string  `json:"id"`
	Email          string  `json:"email"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	LastActivityAt string  `json:"last_activity_at"`
	IdleHours      float64 `json:"idle_hours"`
	Abandoned      bool    `json:"abandoned"`
	CompletedAt    *string `json:"completed_at"`
	TenantID       *string `json:"tenant_id"`
}

type sessionsListResponse struct {
	Data       []sessionRow `json:"data"`
	Pagination pagination   `json:"pagination"`
}

func (h *OnboardingFunnelHandler) sessions(c *gin.Context) {
	p := parseSessionsParams(c)

	res, err := h.client.ListSessions(c.Request.Context(), p)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no data.
	rows := make([]sessionRow, 0, len(res.Sessions))
	for _, s := range res.Sessions {
		rows = append(rows, h.toSessionRow(s))
	}

	c.JSON(http.StatusOK, sessionsListResponse{
		Data: rows,
		Pagination: pagination{
			// pagination.limit/page mirror what the client reported (the
			// clamped, effective values), never the raw request — see
			// parseSessionsParams.
			Page:  max(res.Page, 1),
			Limit: res.Limit,
			Total: res.Total,
		},
	})
}

func (h *OnboardingFunnelHandler) toSessionRow(s onboardingfunnel.Session) sessionRow {
	row := sessionRow{
		ID:             s.ID,
		Email:          s.Email,
		Status:         s.Status,
		CreatedAt:      s.CreatedAt.UTC().Format(time.RFC3339),
		LastActivityAt: s.LastActivityAt.UTC().Format(time.RFC3339),
		IdleHours:      s.IdleHours,
		Abandoned:      s.Abandoned,
	}
	if s.CompletedAt != nil {
		v := s.CompletedAt.UTC().Format(time.RFC3339)
		row.CompletedAt = &v
	}
	if s.TenantID != nil {
		// Bare id, no copy-of-pointer aliasing into the client's struct.
		v := *s.TenantID
		row.TenantID = &v
	}
	return row
}

// parseSessionsParams never errors. A missing parameter takes platform-api's
// default; an oversized limit is clamped there.
func parseSessionsParams(c *gin.Context) onboardingfunnel.SessionsParams {
	p := onboardingfunnel.SessionsParams{
		Status: strings.TrimSpace(c.Query("status")),
	}
	if t, ok := parseTime(c.Query("created_from")); ok {
		p.CreatedFrom = t
	}
	if t, ok := parseTime(c.Query("created_to")); ok {
		p.CreatedTo = t
	}
	if v := strings.TrimSpace(c.Query("abandoned")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			p.Abandoned = &b
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Limit = n
		}
	}
	return p
}

// respondErr maps client errors to the surface's stable codes.
//
// ErrUnavailable becomes 503, never an empty 200: an empty funnel/session
// list and an unreachable upstream are different answers, and a console
// operator shown "no activity" would believe the first. There is no
// ErrNotFound case — neither endpoint 404s.
func (h *OnboardingFunnelHandler) respondErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, onboardingfunnel.ErrUnavailable):
		if h.logger != nil {
			h.logger.Error("onboarding funnel upstream unavailable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "onboarding funnel is unavailable",
		})
	default:
		if h.logger != nil {
			h.logger.Error("onboarding funnel", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the onboarding funnel",
		})
	}
}
