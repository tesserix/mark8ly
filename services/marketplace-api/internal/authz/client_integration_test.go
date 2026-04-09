//go:build integration

// This test requires a running OpenFGA at TEST_FGA_API_URL with a store
// named "mark8ly-platform" whose authorization model includes at minimum
// the `member` relation on `tenant` objects (the platform-api model
// satisfies this). Local devs without that infra get a clean skip.
//
// Per spec §13.1.1 marketplace-api's production code must NOT write
// tuples — the Client interface intentionally omits Write. This test
// file imports the raw openfga-go-sdk directly to seed + clean up a
// test tuple, which is fine because test fixtures are not production
// code.
//
// What the test verifies:
//  1. DiscoverStoreID finds the seeded store.
//  2. A Client built against that store sees a freshly-written tuple.
//  3. An absent user does not.
//  4. The test cleans up its own tuple.
//
// The test uses the literal `member` relation (present in every
// tenant-style FGA model) rather than `owner` + derived-role checks,
// because the marketplace-api-specific role definitions may not be
// loaded into the CI-seeded model yet. If you extend the model later
// you can fold in an owner→admin implication assertion.

package authz

import (
	"context"
	"os"
	"testing"
	"time"

	openfga "github.com/openfga/go-sdk/client"
)

func TestRealFGA_CheckMembership(t *testing.T) {
	apiURL := os.Getenv("TEST_FGA_API_URL")
	if apiURL == "" {
		t.Skip("TEST_FGA_API_URL not set; skipping real-FGA integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	storeID, err := DiscoverStoreID(ctx, apiURL, FGAStoreName)
	if err != nil {
		t.Fatalf("DiscoverStoreID: %v", err)
	}
	if storeID == "" {
		t.Fatalf("store %q not found at %s — bring up openfga-seed first", FGAStoreName, apiURL)
	}

	// Build our (read-only) Client under test.
	c, err := New(Config{APIURL: apiURL, StoreID: storeID})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Seed a test tuple using the raw SDK directly — test fixtures may
	// write; production code may not.
	rawSDK, err := openfga.NewSdkClient(&openfga.ClientConfiguration{
		ApiUrl:  apiURL,
		StoreId: storeID,
	})
	if err != nil {
		t.Fatalf("raw sdk: %v", err)
	}

	// Use a unique tenant id so parallel runs + stale fixtures don't
	// clobber each other.
	tenantID := "marketplace-api-inttest-" + time.Now().UTC().Format("20060102T150405.000000000")
	userID := "marketplace-api-inttest-user"
	relation := "member"

	tuple := openfga.ClientTupleKey{
		User:     "user:" + userID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}

	_, err = rawSDK.Write(ctx).Body(openfga.ClientWriteRequest{
		Writes: []openfga.ClientTupleKey{tuple},
	}).Execute()
	if err != nil {
		t.Fatalf("seed write: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, cleanupErr := rawSDK.Write(cleanupCtx).Body(openfga.ClientWriteRequest{
			Deletes: []openfga.ClientTupleKeyWithoutCondition{{
				User:     tuple.User,
				Relation: tuple.Relation,
				Object:   tuple.Object,
			}},
		}).Execute()
		if cleanupErr != nil {
			t.Logf("cleanup delete: %v", cleanupErr)
		}
	})

	// Happy path — the seeded user is a member.
	allowed, err := c.CheckMembership(ctx, userID, tenantID)
	if err != nil {
		t.Fatalf("CheckMembership: %v", err)
	}
	if !allowed {
		t.Errorf("CheckMembership(%s, %s) = false, want true", userID, tenantID)
	}

	// Generic Check on the literal relation.
	allowed, err = c.Check(ctx, userID, relation, tenantID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !allowed {
		t.Errorf("Check(%s, %s, %s) = false, want true", userID, relation, tenantID)
	}

	// Absent user must not be allowed.
	allowed, err = c.CheckMembership(ctx, "absent-user-"+tenantID, tenantID)
	if err != nil {
		t.Fatalf("CheckMembership absent: %v", err)
	}
	if allowed {
		t.Errorf("CheckMembership(absent, %s) = true, want false", tenantID)
	}
}
