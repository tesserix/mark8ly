package arbitrage

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type spyAuditEmitter struct {
	events []AuditEmittable
}

func (s *spyAuditEmitter) Emit(_ context.Context, evt AuditEmittable) {
	s.events = append(s.events, evt)
}

func TestAuditServicePIILogger_EmitsOnce(t *testing.T) {
	spy := &spyAuditEmitter{}
	logger := AuditServicePIILogger{Emitter: spy, Source: "test"}
	logger.LogPIIAccess(context.Background(), PIIAccessEvent{
		Operation: "test_op",
		TenantID:  uuid.New(),
		StoreID:   uuid.New(),
		Actor:     uuid.New(),
	})
	if len(spy.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(spy.events))
	}
	if spy.events[0].Kind != "arbitrage_pii_access" {
		t.Errorf("Kind=%q, want arbitrage_pii_access", spy.events[0].Kind)
	}
}

func TestNopPIILogger_DoesNotPanic(t *testing.T) {
	var nop NopPIILogger
	nop.LogPIIAccess(context.Background(), PIIAccessEvent{Operation: "test"})
}
