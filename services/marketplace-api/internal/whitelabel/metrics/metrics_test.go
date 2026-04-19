package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestLifecycleTransition_Increments(t *testing.T) {
	// Reset via With(...).Add(0) pattern against the package var — tests
	// share state; that's OK here because the metric is label-keyed.
	before := testutil.ToFloat64(LifecycleTransition.WithLabelValues("sunset_scheduled", "downloads_blocked"))
	LifecycleTransition.WithLabelValues("sunset_scheduled", "downloads_blocked").Inc()
	after := testutil.ToFloat64(LifecycleTransition.WithLabelValues("sunset_scheduled", "downloads_blocked"))
	if after-before != 1 {
		t.Errorf("LifecycleTransition.Inc() moved counter by %v; want 1", after-before)
	}
}

func TestCredentialAccessed_Increments(t *testing.T) {
	before := testutil.ToFloat64(CredentialAccessed.WithLabelValues("apple-asc-api-key"))
	CredentialAccessed.WithLabelValues("apple-asc-api-key").Inc()
	after := testutil.ToFloat64(CredentialAccessed.WithLabelValues("apple-asc-api-key"))
	if after-before != 1 {
		t.Errorf("CredentialAccessed.Inc() moved counter by %v; want 1", after-before)
	}
}

// TestMustRegister_CustomRegistry exercises the public helper so callers
// testing with isolated registries don't hit the duplicate-registration
// panic from the default registry.
func TestMustRegister_CustomRegistry(t *testing.T) {
	reg := prometheus.NewRegistry()
	// Fresh collector instances to avoid re-registering the package-level
	// vars on reg (which would fail because they're already on the default
	// registry).
	local := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_white_label_app_lifecycle_transition_total",
		Help: "test",
	}, []string{"from", "to"})
	reg.MustRegister(local)

	local.WithLabelValues("a", "b").Inc()

	gathered, err := reg.Gather()
	if err != nil {
		t.Fatalf("reg.Gather: %v", err)
	}
	found := false
	for _, mf := range gathered {
		if strings.Contains(mf.GetName(), "lifecycle_transition") {
			found = true
		}
	}
	if !found {
		t.Error("custom registry missing lifecycle_transition metric")
	}
}
