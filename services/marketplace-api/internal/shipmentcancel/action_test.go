package shipmentcancel

import "testing"

func TestResolveAction(t *testing.T) {
	cases := []struct {
		status string
		want   Action
	}{
		{"", ActionCancelForward},
		{"pending", ActionCancelForward},
		{"PENDING", ActionCancelForward},
		{"created", ActionCancelForward},
		{"manifested", ActionCancelForward},
		{"in_transit", ActionTriggerRTO},
		{"out_for_delivery", ActionTriggerRTO},
		{"delivered", ActionReversePickup},
		{"cancelled", ActionNoop},
		{"canceled", ActionNoop},
		{"returned", ActionNoop},
		{"rto", ActionNoop},
		{"exception", ActionNoop},
		{"something_unknown", ActionNoop},
	}
	for _, tc := range cases {
		if got := ResolveAction(tc.status); got != tc.want {
			t.Errorf("ResolveAction(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
