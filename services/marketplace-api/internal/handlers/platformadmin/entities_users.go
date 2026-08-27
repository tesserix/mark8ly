package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/estateuserdir"
)

// EstateUserDirectory is the slice of estateuserdir.Client this handler needs.
type EstateUserDirectory interface {
	List(ctx context.Context, p estateuserdir.ListParams) (*estateuserdir.ListResult, error)
}

// EntitiesUsersHandler serves GET /admin/entities/users (#278).
//
// # Staff and operators, never merchants' end customers
//
// The scope constraint is the important part of #278 and it is structural
// rather than a filter: this handler's only source is platform-api's staff
// directory, which is derived from tenant owners and accepted invitations. It
// has no path to customer_profiles at all. A future end-user lookup is its
// own decision with its own review — the console has a gate built and waiting
// (EstateProduct.endUserLookup), and mark8ly has never declared itself in.
type EntitiesUsersHandler struct {
	dir    EstateUserDirectory
	logger *slog.Logger
}

// NewEntitiesUsersHandler constructs the handler. logger may be nil.
func NewEntitiesUsersHandler(dir EstateUserDirectory, logger *slog.Logger) *EntitiesUsersHandler {
	return &EntitiesUsersHandler{dir: dir, logger: logger}
}

func (h *EntitiesUsersHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/entities/users", h.list)
}

// userRow is the §8.9 entity-row wire shape.
//
// Sublabel is omitempty because §8.9 rejects an empty sublabel as firmly as it
// accepts an absent one: a row with no disambiguator must OMIT the key, or a
// consumer renders a placeholder where it should render nothing. `source` is
// deliberately absent — the platform stamps provenance, and a product-supplied
// one is a forgeable claim.
type userRow struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Sublabel    string `json:"sublabel,omitempty"`
	Email       string `json:"email"`
	Roles       string `json:"roles"`
	TenantCount int64  `json:"tenant_count"`
}

type userListResponse struct {
	Data       []userRow  `json:"data"`
	Pagination pagination `json:"pagination"`
}

func (h *EntitiesUsersHandler) list(c *gin.Context) {
	p := parseUserParams(c)

	res, err := h.dir.List(c.Request.Context(), p)
	if err != nil {
		if errors.Is(err, estateuserdir.ErrUnavailable) {
			// 503, never an empty 200. An empty directory and an unreachable
			// one are different answers, and an operator shown "no users"
			// would believe the first.
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "unavailable", "message": "the estate directory is unavailable",
			})
			return
		}
		if h.logger != nil {
			h.logger.Error("estate user directory failed", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal", "message": "could not load the user directory",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to null, which defeats
	// a caller's `?? []` and crashes their page exactly when there is no data.
	rows := make([]userRow, 0)
	if res != nil {
		for _, u := range res.Users {
			rows = append(rows, toUserRow(u))
		}
	}

	out := userListResponse{Data: rows}
	if res != nil {
		out.Pagination = pagination{Page: max(res.Page, 1), Limit: res.Limit, Total: res.Total}
	}
	if out.Pagination.Page < 1 {
		out.Pagination.Page = max(p.Page, 1)
	}
	c.JSON(http.StatusOK, out)
}

// toUserRow maps a directory row to the §8.9 shape.
//
// The email is the id: identity here is derived from tenant ownership and
// invitations, and the email is the only stable key a person has across both.
func toUserRow(u estateuserdir.User) userRow {
	return userRow{
		ID:          u.Email,
		Label:       u.Email,
		Sublabel:    userSublabel(u),
		Email:       u.Email,
		Roles:       u.Roles,
		TenantCount: u.TenantCount,
	}
}

// userSublabel disambiguates a person by where they work.
//
// Empty when there is nothing useful to say, and userRow.Sublabel is
// omitempty, so §8.9's "omit rather than send empty" holds. No person name is
// recorded anywhere in this estate today — tenants.name is a business name —
// so the tenant is the only disambiguator available.
func userSublabel(u estateuserdir.User) string {
	name := strings.TrimSpace(u.TenantName)
	if name == "" {
		return ""
	}
	if u.TenantCount > 1 {
		return name + " +" + strconv.FormatInt(u.TenantCount-1, 10) + " more"
	}
	return name
}

func parseUserParams(c *gin.Context) estateuserdir.ListParams {
	p := estateuserdir.ListParams{Q: strings.TrimSpace(c.Query("q"))}
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
