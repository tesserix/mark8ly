package public

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/journal"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// journalUnsubscriber is the slice of *journal.Repository this handler
// needs. Accepting the interface (rather than the concrete type) keeps
// the handler testable without a database.
type journalUnsubscriber interface {
	Unsubscribe(token string) error
}

// journalUnsubscribeInput is the JSON binding struct for the public
// unsubscribe endpoint. No `binding:"required"` on Token: a missing or
// empty token is handled the same as any other unrecognised token (see
// Unsubscribe below), not rejected with a 400 — a 400 vs 200 split would
// itself be a (weak) signal for an attacker probing for well-formed vs.
// malformed tokens.
type journalUnsubscribeInput struct {
	Token string `json:"token"`
}

// JournalUnsubscribeHandler serves the public, tenant-free erasure path
// for a Journal subscriber (#153) — the mechanism that makes good on the
// customererasure declaredExclusions promise that these addresses "still
// carry an art.17 right, exercised against the platform" even though the
// table has no store_id/tenant_id for a merchant-scoped erasure to reach.
type JournalUnsubscribeHandler struct {
	repo    journalUnsubscriber
	limiter *journal.RateLimiter
	logger  *slog.Logger
}

// NewJournalUnsubscribeHandler constructs a JournalUnsubscribeHandler.
func NewJournalUnsubscribeHandler(repo journalUnsubscriber, limiter *journal.RateLimiter, logger *slog.Logger) *JournalUnsubscribeHandler {
	return &JournalUnsubscribeHandler{repo: repo, limiter: limiter, logger: logger}
}

// Unsubscribe handles POST /journal/unsubscribe. Anonymous by design —
// the token itself, mailed to the subscriber out of band, is the only
// credential required.
//
// Always returns 200, whether the token matched a row, matched nothing,
// was already used, or was malformed/empty — the response must never
// reveal whether a given token (or, transitively, an email address) was
// ever valid. The only path that deviates is a genuine backend failure
// (db unreachable, etc.), which surfaces as a 500 exactly like the
// sibling Subscribe handler, since that failure mode carries no
// enumeration signal about any particular token.
func (h *JournalUnsubscribeHandler) Unsubscribe(c *gin.Context) {
	if h.limiter != nil && !h.limiter.Allow(c.ClientIP()) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, journalErrEnvelope(
			apperrors.CodeRateLimited, "too many requests, please try again later"))
		return
	}

	var req journalUnsubscribeInput
	// A body that fails to bind (missing/non-string token, malformed
	// JSON) or a token that isn't even the right shape is treated
	// exactly like an unrecognised token: 200, no repository call at
	// all — so a malformed token never even reaches the database, let
	// alone gets logged. (The repository re-checks the length itself
	// as defense in depth for any other caller.) Never log req.Token
	// or any derived email.
	if err := c.ShouldBindJSON(&req); err != nil ||
		len(req.Token) != journal.UnsubscribeTokenLength {
		c.JSON(http.StatusOK, gin.H{"unsubscribed": true})
		return
	}

	if err := h.repo.Unsubscribe(req.Token); err != nil {
		if h.logger != nil {
			h.logger.Error("journal unsubscribe failed", slog.String("err", err.Error()))
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, journalErrEnvelope(
			"internal", "internal server error"))
		return
	}

	c.JSON(http.StatusOK, gin.H{"unsubscribed": true})
}
