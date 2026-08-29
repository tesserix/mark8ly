package main

import (
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
)

// tenantGateInvalidator converts a possibly-nil *tenantgate.Gate into the
// interface platformadmin.Deps wants, returning an explicitly nil
// interface when the gate is unwired (#341).
//
// Assigning the *Gate directly is the bug this exists to prevent: a nil
// pointer stored in an interface makes the INTERFACE non-nil, so
// `if h.invalidate != nil` in tenant_lifecycle.go always fires and calls
// Invalidate on a nil receiver. That is safe today only because
// (*Gate).Invalidate checks its own receiver — an accident of that one
// implementation, not a property of the interface. A guard that cannot
// fail is indistinguishable from no guard, and reads to the next author
// as proof the case is handled.
//
// This mirrors what main.go already does for Deps.TenantGate, where the
// same shape was caught during #287's review.
func tenantGateInvalidator(g *tenantgate.Gate) platformadmin.TenantGateInvalidator {
	if g == nil {
		return nil
	}
	return g
}
