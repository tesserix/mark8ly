package platformadmin

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// BreakGlassLister is the subset of the cross-tenant break-glass read this
// handler needs. Narrowed to one method for the same reason as
// EmailSendLister.
type BreakGlassLister interface {
	ListPlatform(ctx context.Context, db *gorm.DB, f breakglass.PlatformListFilter,
		asOf time.Time) (breakglass.PlatformListResult, error)
}

// BreakGlassListerFunc adapts a plain function, so breakglass.ListPlatform —
// a package function, not a method — can be wired directly in main.go. Same
// pattern as EmailSendListerFunc.
type BreakGlassListerFunc func(ctx context.Context, db *gorm.DB,
	f breakglass.PlatformListFilter, asOf time.Time) (breakglass.PlatformListResult, error)

func (fn BreakGlassListerFunc) ListPlatform(ctx context.Context, db *gorm.DB,
	f breakglass.PlatformListFilter, asOf time.Time) (breakglass.PlatformListResult, error) {
	return fn(ctx, db, f, asOf)
}

// BreakGlassHandler serves GET /admin/break-glass — the cross-tenant
// emergency-account inventory (#333).
//
// break_glass_accounts is a deliberate authentication bypass; before this
// endpoint the only control on its use was rotation, because nothing outside
// mark8ly could see when an emergency credential was used. This is the read
// that closes that gap — and only the read: it never touches secret_path,
// password_hash or totp_secret_ref, which breakglass.ListPlatform already
// excludes at the query layer, and breakGlassRow below excludes again at the
// wire layer.
type BreakGlassHandler struct {
	db     *gorm.DB
	repo   BreakGlassLister
	dir    TenantDirectory
	logger *slog.Logger
	now    func() time.Time
}

// NewBreakGlassHandler constructs the handler. logger may be nil.
func NewBreakGlassHandler(db *gorm.DB, repo BreakGlassLister, dir TenantDirectory, logger *slog.Logger) *BreakGlassHandler {
	return &BreakGlassHandler{db: db, repo: repo, dir: dir, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group. The read is additionally
// gated on an exact `rotate-credentials` platform capability — see
// RequiredReadCapabilities in middleware.go — but that gate is enforced by
// middleware ahead of this handler, not here.
func (h *BreakGlassHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/break-glass", h.List)
}

// breakGlassRow is the pinned contract shape (see the plan's Contract
// section). Populated field by field from breakglass.PlatformRow — never
// embedded, never re-marshalled — so a column added to that struct tomorrow
// cannot reach the wire without a deliberate edit here.
type breakGlassRow struct {
	TenantID   string `json:"tenant_id"`
	TenantName string `json:"tenant_name,omitempty"`
	// TOTPEnrolled mirrors PlatformRow.TOTPEnrolled.
	TOTPEnrolled bool `json:"totp_enrolled"`
	// LastUsedAt is nullable: a never-used account has this absent.
	LastUsedAt    *string `json:"last_used_at"`
	LastRotatedAt string  `json:"last_rotated_at"`
	// RotationScheduledAt is nullable.
	RotationScheduledAt *string `json:"rotation_scheduled_at"`
	// LockedOut means "at least one break_glass_lockouts row currently
	// names this tenant" — see breakglass.PlatformRow.LockedOut for the
	// per-IP, not per-account, caveat this does NOT repeat on the wire
	// because the contract row does not carry prose.
	LockedOut bool `json:"locked_out"`
	// LockoutExpiresAt is nullable, and nil whenever LockedOut is false.
	LockoutExpiresAt *string `json:"lockout_expires_at"`
	CreatedAt        string  `json:"created_at"`
}

type breakGlassListResponse struct {
	Data       []breakGlassRow `json:"data"`
	Pagination pagination      `json:"pagination"`
}

// List serves GET /admin/break-glass.
func (h *BreakGlassHandler) List(c *gin.Context) {
	f, asOf := h.parseFilter(c)

	res, err := h.repo.ListPlatform(c.Request.Context(), h.db, f, asOf)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform break-glass list failed", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "could not load the break-glass inventory",
		})
		return
	}

	names := h.lookupTenantNames(c.Request.Context(), res.Accounts)

	// Allocate before appending: a nil slice marshals to null, which defeats
	// a caller's `?? []` exactly when there is no data.
	rows := make([]breakGlassRow, 0, len(res.Accounts))
	for _, a := range res.Accounts {
		rows = append(rows, toBreakGlassRow(a, names))
	}

	c.JSON(http.StatusOK, breakGlassListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(f.Page, 1),
			Limit: effectiveBreakGlassLimit(f.Limit),
			Total: res.Total,
		},
	})
}

// lookupTenantNames collects the DISTINCT tenant ids on the page and makes
// exactly one tenantdirectory.List call for all of them, never one call per
// row. A page with zero rows makes no call at all: an empty IDs slice would
// otherwise match "no id filter" on the wire and pull back the whole
// directory instead of nothing. Mirrors
// BillingSubscriptionsHandler.lookupTenantNames — with one deliberate
// difference: this surface is a security control, and a platform-api outage
// must not hide it. A directory error therefore DEGRADES to rows still
// identified by tenant_id with the name OMITTED (dropped by omitempty),
// rather than failing the request — and never to a fabricated name, which
// would read as real and is worse than none.
func (h *BreakGlassHandler) lookupTenantNames(ctx context.Context, accounts []breakglass.PlatformRow) map[string]string {
	names := map[string]string{}
	if len(accounts) == 0 {
		return names
	}

	seen := make(map[string]bool, len(accounts))
	ids := make([]string, 0, len(accounts))
	for _, a := range accounts {
		id := a.TenantID.String()
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	res, err := h.dir.List(ctx, tenantdirectory.ListParams{IDs: ids, Limit: len(ids)})
	if err != nil {
		// Degrade to rows identified by tenant_id with the name OMITTED —
		// NOT a fabricated name. The console renders tenant_name as a
		// display string, so putting the raw uuid there would read as a
		// real (if odd) name rather than "unknown", which is worse than
		// omitting it. The request still returns 200: the whole point of
		// deviating from BillingSubscriptionsHandler here is that a
		// platform-api outage must not hide a security control, and that
		// is satisfied by returning the rows with ids and no names.
		if h.logger != nil {
			h.logger.Warn("break-glass: tenant directory unavailable, omitting names", "err", err)
		}
		return names
	}

	for _, t := range res.Tenants {
		names[t.ID] = t.Name
	}

	// A tenant id present on a break-glass row but absent from the
	// directory response means the two services disagree about which
	// tenants exist. The row still appears with its name simply omitted
	// (never a fabricated fallback), but this is worth a log line.
	for _, id := range ids {
		if _, ok := names[id]; !ok && h.logger != nil {
			h.logger.Warn("break-glass: tenant missing from directory", "tenant_id", id)
		}
	}

	return names
}

func toBreakGlassRow(a breakglass.PlatformRow, names map[string]string) breakGlassRow {
	tenantID := a.TenantID.String()
	row := breakGlassRow{
		TenantID:      tenantID,
		TenantName:    names[tenantID],
		TOTPEnrolled:  a.TOTPEnrolled,
		LastRotatedAt: a.LastRotatedAt.UTC().Format(time.RFC3339),
		LockedOut:     a.LockedOut,
		CreatedAt:     a.CreatedAt.UTC().Format(time.RFC3339),
	}
	if a.LastUsedAt != nil {
		v := a.LastUsedAt.UTC().Format(time.RFC3339)
		row.LastUsedAt = &v
	}
	if a.RotationScheduledAt != nil {
		v := a.RotationScheduledAt.UTC().Format(time.RFC3339)
		row.RotationScheduledAt = &v
	}
	if a.LockoutExpiresAt != nil {
		v := a.LockoutExpiresAt.UTC().Format(time.RFC3339)
		row.LockoutExpiresAt = &v
	}
	return row
}

// parseFilter never errors: a malformed parameter takes the default, matching
// every other read on this surface.
func (h *BreakGlassHandler) parseFilter(c *gin.Context) (breakglass.PlatformListFilter, time.Time) {
	f := breakglass.PlatformListFilter{
		Page:     parsePositiveIntDefault(c.Query("page"), 1),
		Limit:    parsePositiveIntDefault(c.Query("limit"), 0),
		SortDesc: parseBreakGlassSort(c.Query("sort")),
	}

	if raw := strings.TrimSpace(c.Query("tenant_id")); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			f.TenantID = &id
		}
	}
	if t, ok := parseBreakGlassTime(c.Query("used_after")); ok {
		f.UsedAfter = &t
	}
	if t, ok := parseBreakGlassTime(c.Query("used_before")); ok {
		f.UsedBefore = &t
	}
	if b, ok := parseBreakGlassBool(c.Query("used")); ok {
		f.Used = &b
	}
	if b, ok := parseBreakGlassBool(c.Query("locked")); ok {
		f.Locked = &b
	}

	return f, h.now().UTC()
}

// parseBreakGlassSort accepts only "last_used_at" and "-last_used_at";
// anything else — including an absent or unknown value — falls back to the
// default descending sort, never erroring.
func parseBreakGlassSort(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "last_used_at":
		return false
	default:
		return true
	}
}

func parseBreakGlassTime(v string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseBreakGlassBool(v string) (bool, bool) {
	switch strings.TrimSpace(v) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

// effectiveBreakGlassLimit reports the limit actually applied, so
// total/limit is a correct page count even when the caller asked for more
// than the ceiling.
func effectiveBreakGlassLimit(limit int) int {
	switch {
	case limit <= 0:
		return breakglass.DefaultPlatformPageSize
	case limit > breakglass.MaxPlatformPageSize:
		return breakglass.MaxPlatformPageSize
	default:
		return limit
	}
}
