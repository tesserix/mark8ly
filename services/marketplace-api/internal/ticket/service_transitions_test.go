package ticket

import "testing"

// TestCustomerAllowedStatusTransition guards the customer-side state
// machine. The matrix here is the wire contract the storefront UI
// relies on: any change here must be paired with an UI update in
// apps/storefront/app/account/tickets/[id]/StatusActions.tsx, otherwise
// the shopper sees a button that the backend will reject with 409.
func TestCustomerAllowedStatusTransition(t *testing.T) {
	cases := []struct {
		name     string
		from, to TicketStatus
		want     bool
	}{
		{"open→resolved", TicketStatusOpen, TicketStatusResolved, true},
		{"open→closed", TicketStatusOpen, TicketStatusClosed, true},
		{"open→in_progress (merchant only)", TicketStatusOpen, TicketStatusInProgress, false},
		{"open→open (no-op)", TicketStatusOpen, TicketStatusOpen, false},

		{"in_progress→resolved", TicketStatusInProgress, TicketStatusResolved, true},
		{"in_progress→closed", TicketStatusInProgress, TicketStatusClosed, true},
		{"in_progress→open (merchant only)", TicketStatusInProgress, TicketStatusOpen, false},

		{"resolved→open (reopen)", TicketStatusResolved, TicketStatusOpen, true},
		{"resolved→closed", TicketStatusResolved, TicketStatusClosed, true},
		{"resolved→in_progress (merchant only)", TicketStatusResolved, TicketStatusInProgress, false},
		{"resolved→resolved (no-op)", TicketStatusResolved, TicketStatusResolved, false},

		{"closed→anything (terminal)", TicketStatusClosed, TicketStatusOpen, false},
		{"closed→resolved", TicketStatusClosed, TicketStatusResolved, false},
		{"closed→in_progress", TicketStatusClosed, TicketStatusInProgress, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CustomerAllowedStatusTransition(tc.from, tc.to)
			if got != tc.want {
				t.Errorf("CustomerAllowedStatusTransition(%q, %q) = %v, want %v",
					tc.from, tc.to, got, tc.want)
			}
		})
	}
}
