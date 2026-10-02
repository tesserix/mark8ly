// Internal-package test: the decisions Reap makes are unexported, and the
// point of the seam on checkRefs is to assert them without a database.
// The queries themselves are covered by the tenantpurge integration tests.
package blobreap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/media"
)

const bucket = "mark8ly-prod-media"

func url(key string) string { return "https://storage.googleapis.com/" + bucket + "/" + key }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTestReaper builds a Reaper whose reference check answers from `live`.
func newTestReaper(t *testing.T, live ...string) (*Reaper, *media.FakeUploader) {
	t.Helper()
	fake := media.NewFakeUploader()
	r := &Reaper{deleter: fake, bucket: bucket, logger: quiet(), max: DefaultMaxObjects}
	set := make(map[string]struct{}, len(live))
	for _, k := range live {
		set[k] = struct{}{}
	}
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) { return set, nil }
	return r, fake
}

func TestReap_DeletesOurUnreferencedObjects(t *testing.T) {
	r, fake := newTestReaper(t)
	out := r.Reap(context.Background(), []string{
		url("tenants/t1/products/media/aaa/one.jpg"),
		"tenants/t1/products/media/bbb/two.jpg", // bare key, same gate
	})
	require.Equal(t, Outcome{Deleted: 2}, out)
	require.ElementsMatch(t, []string{
		"tenants/t1/products/media/aaa/one.jpg",
		"tenants/t1/products/media/bbb/two.jpg",
	}, fake.DeletedKeys())
}

// The reason this package exists rather than reusing the erasure helper.
//
// product_media.storage_key is content-addressed and copy-to-store points
// several rows at one object. A purged tenant's rows disappearing does
// not make the object unreferenced — deleting it would punch a hole in a
// surviving tenant's catalogue.
func TestReap_LeavesAnObjectASurvivingRowStillReferences(t *testing.T) {
	shared := "tenants/t1/products/media/shared/hero.jpg"
	r, fake := newTestReaper(t, shared)

	out := r.Reap(context.Background(), []string{
		url(shared),
		url("tenants/t1/products/media/solo/only.jpg"),
	})
	require.Equal(t, Outcome{Deleted: 1, SkippedStillReferenced: 1}, out)
	require.ElementsMatch(t, []string{"tenants/t1/products/media/solo/only.jpg"}, fake.DeletedKeys())
}

func TestReap_RefusesObjectsThatAreNotOurs(t *testing.T) {
	r, fake := newTestReaper(t)
	out := r.Reap(context.Background(), []string{
		"https://storage.googleapis.com/someone-elses-bucket/tenants/t1/x.jpg",
		"https://cdn.example.com/avatar.png",
		url("public/brochure.pdf"), // our bucket, a prefix we never mint
		"not-a-url-or-an-owned-key",
		"https://merchant.example.com/webhooks/orders", // a webhook endpoint
	})
	require.Equal(t, Outcome{SkippedNotOurs: 5}, out)
	require.Empty(t, fake.DeletedKeys())
}

func TestReap_CollapsesTheThreeColumnsNamingOneObject(t *testing.T) {
	// product_media contributes url, storage_key and gcs_path_original
	// for a single object. Counting it three times would overstate the
	// destruction in the report.
	key := "tenants/t1/products/media/abc/photo.jpg"
	r, fake := newTestReaper(t)
	out := r.Reap(context.Background(), []string{url(key), key, key})
	require.Equal(t, Outcome{Deleted: 1}, out)
	require.Equal(t, 1, fake.Deleted(key))
}

func TestReap_FailsClosedWhenItCannotTellWhatSurvives(t *testing.T) {
	// Not knowing what survives is not permission to delete. A guard that
	// errored and deleted anyway would destroy another tenant's
	// catalogue, which is the whole thing this prevents.
	fake := media.NewFakeUploader()
	r := &Reaper{deleter: fake, bucket: bucket, logger: quiet(), max: DefaultMaxObjects}
	r.checkRefs = func(context.Context, []string) (map[string]struct{}, error) {
		return nil, errors.New("connection reset")
	}
	out := r.Reap(context.Background(), []string{url("tenants/t1/a.jpg"), url("tenants/t1/b.jpg")})
	require.Equal(t, Outcome{Failed: 2}, out)
	require.Empty(t, fake.DeletedKeys(), "a failed guard must destroy nothing")
}

func TestReap_CountsDeleteFailuresWithoutAbortingTheRest(t *testing.T) {
	r, _ := newTestReaper(t)
	bad := "tenants/t1/boom.jpg"
	r.deleter = failingDeleter{bad: bad}
	out := r.Reap(context.Background(), []string{
		url("tenants/t1/ok1.jpg"), url(bad), url("tenants/t1/ok2.jpg"),
	})
	require.Equal(t, Outcome{Deleted: 2, Failed: 1}, out)
}

func TestReap_CapsOneRunAndReportsTheRemainder(t *testing.T) {
	r, fake := newTestReaper(t)
	r.max = 2
	out := r.Reap(context.Background(), []string{
		url("tenants/t1/a.jpg"), url("tenants/t1/b.jpg"),
		url("tenants/t1/c.jpg"), url("tenants/t1/d.jpg"),
	})
	require.Equal(t, Outcome{Deleted: 2, Unreaped: 2}, out)
	require.Len(t, fake.DeletedKeys(), 2)
}

func TestReap_WithNoBucketWiredReportsSkippedRatherThanSilence(t *testing.T) {
	// A deployment that cannot delete objects must say so. Reporting a
	// clean zero is indistinguishable from "there was nothing to delete",
	// which is the pre-#961 lie.
	var r *Reaper
	out := r.Reap(context.Background(), []string{url("tenants/t1/a.jpg")})
	require.Equal(t, Outcome{SkippedNotOurs: 1}, out)

	r2 := &Reaper{logger: quiet(), max: DefaultMaxObjects} // no deleter, no bucket
	out2 := r2.Reap(context.Background(), []string{url("tenants/t1/a.jpg")})
	require.Equal(t, Outcome{SkippedNotOurs: 1}, out2)
}

func TestReap_NothingToDoIsNotAnOutcome(t *testing.T) {
	r, _ := newTestReaper(t)
	require.True(t, r.Reap(context.Background(), nil).Empty())
}

type failingDeleter struct{ bad string }

func (f failingDeleter) Delete(_ context.Context, key string) error {
	if key == f.bad {
		return errors.New("gcs: permission denied")
	}
	return nil
}
