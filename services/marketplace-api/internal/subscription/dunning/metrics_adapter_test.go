package dunning_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/mark8ly/marketplace-api/internal/subscription/dunning"
)

func TestWrapPrometheusCounter_Inc(t *testing.T) {
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_wrap_counter_total",
		Help: "test counter for WrapPrometheusCounter",
	})

	inc := dunning.WrapPrometheusCounter(c)
	inc.Inc()
	inc.Inc()

	assert.Equal(t, float64(2), testutil.ToFloat64(c))
}

func TestWrapPrometheusCounterVec_WithDay_Inc(t *testing.T) {
	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_wrap_counter_vec_total",
		Help: "test counter vec for WrapPrometheusCounterVec",
	}, []string{"day"})

	vec := dunning.WrapPrometheusCounterVec(cv)
	vec.WithDay("day_5").Inc()
	vec.WithDay("day_5").Inc()
	vec.WithDay("day_7").Inc()

	assert.Equal(t, float64(2), testutil.ToFloat64(cv.WithLabelValues("day_5")))
	assert.Equal(t, float64(1), testutil.ToFloat64(cv.WithLabelValues("day_7")))
}
