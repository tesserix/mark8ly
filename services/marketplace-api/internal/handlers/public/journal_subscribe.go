package public

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/journal"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// journalSubscriber is the slice of *journal.Repository this handler
// needs. Accepting the interface (rather than the concrete type) keeps
// the handler testable without a database.
type journalSubscriber interface {
	Subscribe(email, source string) error
}

// JournalSubscribeHandler serves the public, tenant-free Journal email
// capture endpoint behind "Notify me when the first piece goes up" on
// mark8ly.com/blog (#153). It is mounted by RegisterPublic rather than
// under the tenant-scoped storefront/admin route groups — see
// migrations/000124_journal_subscribers.up.sql for why a Journal
// subscriber has no tenant_id to route through in the first place.
type JournalSubscribeHandler struct {
	repo    journalSubscriber
	limiter *journal.RateLimiter
	logger  *slog.Logger
}

// NewJournalSubscribeHandler constructs a JournalSubscribeHandler.
func NewJournalSubscribeHandler(repo journalSubscriber, limiter *journal.RateLimiter, logger *slog.Logger) *JournalSubscribeHandler {
	return &JournalSubscribeHandler{repo: repo, limiter: limiter, logger: logger}
}

// Subscribe handles POST /journal/subscribe. Anonymous and unauthenticated
// by design — it is called from the onboarding app's server on behalf of
// an anonymous visitor of the public marketing site, never from a
// logged-in session. Rate limited by client IP since it is a public,
// unauthenticated write endpoint that will be scraped and spammed.
//
// Always returns 200 for a syntactically valid email — including one
// that is already subscribed — so the response never leaks whether an
// address was previously seen (see #153 requirements).
func (h *JournalSubscribeHandler) Subscribe(c *gin.Context) {
	if h.limiter != nil && !h.limiter.Allow(c.ClientIP()) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, journalErrEnvelope(
			apperrors.CodeRateLimited, "too many requests, please try again later"))
		return
	}

	var req journal.SubscribeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, journalErrEnvelope(
			apperrors.CodeValidationFailed, "a valid email address is required"))
		return
	}

	if err := h.repo.Subscribe(req.Email, journal.SourceJournal); err != nil {
		if h.logger != nil {
			h.logger.Error("journal subscribe failed",
				slog.String("err", err.Error()))
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, journalErrEnvelope(
			"internal", "internal server error"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"subscribed": true})
}

func journalErrEnvelope(code apperrors.Code, msg string) gin.H {
	return gin.H{"error": string(code), "message": msg}
}
