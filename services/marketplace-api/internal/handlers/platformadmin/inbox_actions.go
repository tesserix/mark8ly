package platformadmin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// HeaderIdempotencyKey carries the client's retry token for destructive
// actions. Named after the widely-used convention rather than an X- prefix,
// which RFC 6648 deprecated.
const HeaderIdempotencyKey = "Idempotency-Key"

// maxIdempotencyKeyLen bounds what is stored as a primary key. Generous
// enough for a UUID or a ULID with a client prefix, bounded so a hostile or
// buggy caller cannot write arbitrarily large rows.
const maxIdempotencyKeyLen = 200

// InboxItemSource reads one inbox item back by kind and id. Satisfied by
// *inbox.Aggregator; declared here as a local interface for the same reason
// every other dependency in this package is — the handler is testable without
// a database, and this package does not depend on the aggregator's concrete
// type.
type InboxItemSource interface {
	Get(ctx context.Context, kind, id string) (inbox.Item, error)
}

// InboxActionResult is what an executor reports back, so the handler can
// attribute the audit row without a second lookup.
type InboxActionResult struct {
	// TenantID attributes the audit event. Required: EmitOperatorAction
	// refuses uuid.Nil rather than silently dropping the event (#310).
	TenantID uuid.UUID
	StoreID  *uuid.UUID
	// Status is the item's state after the action, echoed to the caller.
	Status string
}

// InboxActionExecutor performs one kind's actions.
//
// Kinds without an executor are NOT an error in this package — they are
// answered 501. A queue can legitimately be readable before it is actionable,
// and mark8ly has several: sea_manual_review's underlying SEA support is only
// partially implemented, and wiring a one-click approve into half-built
// behaviour is worse than an honest "not implemented" (#281a).
type InboxActionExecutor interface {
	Kind() string
	// Execute runs actionID against item on behalf of operatorID. notes is
	// the optional free-text reason from the request body.
	Execute(ctx context.Context, item inbox.Item, actionID, operatorID, notes string) (InboxActionResult, error)
}

// InboxActionsHandler serves POST /admin/inbox/:kind/:id/actions/:actionId.
type InboxActionsHandler struct {
	src       InboxItemSource
	executors map[string]InboxActionExecutor
	idem      InboxActionIdempotency
	emitter   *audit.Emitter
	logger    *slog.Logger
}

// NewInboxActionsHandler constructs the handler. logger and emitter may be nil.
func NewInboxActionsHandler(
	src InboxItemSource,
	executors []InboxActionExecutor,
	idem InboxActionIdempotency,
	emitter *audit.Emitter,
	logger *slog.Logger,
) *InboxActionsHandler {
	byKind := make(map[string]InboxActionExecutor, len(executors))
	for _, e := range executors {
		if e != nil {
			byKind[e.Kind()] = e
		}
	}
	return &InboxActionsHandler{src: src, executors: byKind, idem: idem, emitter: emitter, logger: logger}
}

func (h *InboxActionsHandler) Register(g *gin.RouterGroup) {
	g.POST("/admin/inbox/:kind/:id/actions/:actionId", h.execute)
}

type inboxActionRequest struct {
	Notes string `json:"notes"`
}

type inboxActionResponse struct {
	Kind     string `json:"kind"`
	ItemID   string `json:"item_id"`
	ActionID string `json:"action_id"`
	Status   string `json:"status"`
	// Replayed marks an answer served from the idempotency record rather than
	// a fresh execution. The console can render "already done" rather than
	// implying it just happened.
	Replayed bool `json:"replayed"`
}

func (h *InboxActionsHandler) execute(c *gin.Context) {
	kind := c.Param("kind")
	itemID := c.Param("id")
	actionID := c.Param("actionId")

	item, err := h.src.Get(c.Request.Context(), kind, itemID)
	if err != nil {
		h.respondLookupErr(c, err)
		return
	}

	// The item's OWN declaration is the contract, not the executor registry:
	// an action mark8ly implements for this kind but did not offer on this
	// item must still be refused, or the declared list is documentation.
	action, ok := declaredAction(item, actionID)
	if !ok {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "undeclared_action",
			"message": "this item does not declare action " + actionID,
		})
		return
	}

	var body inboxActionRequest
	// A malformed or absent body leaves notes empty; notes are optional and a
	// bad body is not a reason to refuse an otherwise valid action.
	_ = c.ShouldBindJSON(&body)

	key := strings.TrimSpace(c.GetHeader(HeaderIdempotencyKey))
	if action.Destructive && key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "idempotency_key_required",
			"message": "destructive actions require an " + HeaderIdempotencyKey + " header",
		})
		return
	}
	if len(key) > maxIdempotencyKeyLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "idempotency_key_too_long",
			"message": "idempotency key exceeds the maximum length",
		})
		return
	}

	operatorID, _ := c.Get(CtxOperatorID)
	operator, _ := operatorID.(string)

	exec, hasExec := h.executors[kind]
	if !hasExec {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "action_not_implemented",
			"message": "no action executor is wired for kind " + kind,
		})
		return
	}

	// Claim BEFORE executing. Claiming afterwards would leave a crash between
	// the write and the record indistinguishable from a request that never
	// ran, which is the exact double-fire this key exists to prevent.
	if key != "" && h.idem != nil {
		first, existing, err := h.idem.Claim(c.Request.Context(), InboxActionRecord{
			Key: key, Kind: kind, ItemID: itemID, ActionID: actionID,
			OperatorID: operator,
			ExpiresAt:  time.Now().UTC().Add(InboxActionIdempotencyTTL),
		})
		if err != nil {
			h.logf("inbox action idempotency claim failed", "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "unavailable",
				"message": "could not verify the idempotency key",
			})
			return
		}
		if !first {
			h.respondReplay(c, kind, itemID, actionID, existing)
			return
		}
	}

	res, err := exec.Execute(c.Request.Context(), item, actionID, operator, body.Notes)
	if err != nil {
		h.respondExecErr(c, err)
		return
	}

	if err := EmitOperatorAction(c, h.emitter, res.TenantID, audit.Event{
		Action:       "inbox." + kind + "." + actionID,
		ResourceType: "inbox_item",
		ResourceID:   itemID,
	}); err != nil {
		// The write already happened. Refusing now would tell the operator it
		// did not, which is worse than a loud log plus an honest success.
		h.logf("inbox action audit emit failed", "err", err, "kind", kind, "item_id", itemID)
	}

	out := inboxActionResponse{Kind: kind, ItemID: itemID, ActionID: actionID, Status: res.Status}
	if key != "" && h.idem != nil {
		if encoded, mErr := json.Marshal(out); mErr == nil {
			if cErr := h.idem.Complete(c.Request.Context(), key, encoded); cErr != nil {
				h.logf("inbox action idempotency complete failed", "err", cErr, "key_present", true)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// declaredAction finds actionID in the item's own declaration.
func declaredAction(item inbox.Item, actionID string) (inbox.Action, bool) {
	for _, a := range item.Actions {
		if a.ID == actionID {
			return a, true
		}
	}
	return inbox.Action{}, false
}

func (h *InboxActionsHandler) respondReplay(c *gin.Context, kind, itemID, actionID string, rec *InboxActionRecord) {
	out := inboxActionResponse{Kind: kind, ItemID: itemID, ActionID: actionID, Replayed: true}
	if rec != nil && len(rec.Outcome) > 0 {
		var stored inboxActionResponse
		if err := json.Unmarshal(rec.Outcome, &stored); err == nil && stored.ActionID != "" {
			stored.Replayed = true
			out = stored
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *InboxActionsHandler) respondLookupErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, inbox.ErrItemNotFound), errors.Is(err, inbox.ErrUnknownKind):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "no such inbox item",
		})
	case errors.Is(err, inbox.ErrGetNotSupported):
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":   "action_not_implemented",
			"message": "this kind cannot read back a single item, so its actions cannot be validated",
		})
	default:
		h.logf("inbox action item lookup failed", "err", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "unavailable", "message": "could not load the inbox item",
		})
	}
}

func (h *InboxActionsHandler) respondExecErr(c *gin.Context, err error) {
	// A vanished or already-decided item is the common race: two operators
	// acting on the same queue row. 409 says "someone got there first",
	// which is actionable, where 500 is not.
	if errors.Is(err, inbox.ErrItemNotFound) {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "already_actioned",
			"message": "this item is no longer waiting on a decision",
		})
		return
	}
	h.logf("inbox action execution failed", "err", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": "internal", "message": "the action could not be completed",
	})
}

func (h *InboxActionsHandler) logf(msg string, args ...any) {
	if h.logger != nil {
		h.logger.Error(msg, args...)
		return
	}
	slog.Default().Error(msg, args...)
}
