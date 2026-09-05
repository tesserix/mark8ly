package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// The GIP-retirement decision (#708) reads this counter, so the metric
// name, the label name and the label VALUES are a contract with whoever
// writes that query — not incidental strings.
func TestMobileAdminTokenVerifiedTotal_CountsPerIssuer(t *testing.T) {
	MobileAdminTokenVerifiedTotal.Reset()

	MobileAdminTokenVerifiedTotal.WithLabelValues("zitadel").Inc()
	MobileAdminTokenVerifiedTotal.WithLabelValues("zitadel").Inc()
	MobileAdminTokenVerifiedTotal.WithLabelValues("gip").Inc()

	const expected = `
# HELP mobile_admin_token_verified_total Count of successful mobile-admin bearer token verifications, labeled by the issuer that accepted the token (zitadel | gip). Successes only — a rejected token is attributed to no issuer. Used to decide when GIP has drained and can be retired (#708).
# TYPE mobile_admin_token_verified_total counter
mobile_admin_token_verified_total{issuer="gip"} 1
mobile_admin_token_verified_total{issuer="zitadel"} 2
`
	if err := testutil.CollectAndCompare(
		MobileAdminTokenVerifiedTotal,
		strings.NewReader(expected),
		"mobile_admin_token_verified_total",
	); err != nil {
		t.Fatalf("unexpected metric output: %v", err)
	}
}
