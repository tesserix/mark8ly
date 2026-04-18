package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestMatrix_StoresLimits(t *testing.T) {
	require.Equal(t, 1, plangate.Limit(subscription.PlanTrial, plangate.FeatureStores))
	require.Equal(t, 2, plangate.Limit(subscription.PlanStarter, plangate.FeatureStores))
	require.Equal(t, 5, plangate.Limit(subscription.PlanStudio, plangate.FeatureStores))
	require.Equal(t, 10, plangate.Limit(subscription.PlanPro, plangate.FeatureStores))
}

func TestMatrix_ImagesPerProduct_Grandfathering(t *testing.T) {
	require.Equal(t, 25, plangate.Limit(subscription.PlanTrial, plangate.FeatureImagesPerProduct))
	require.Equal(t, 25, plangate.Limit(subscription.PlanStarter, plangate.FeatureImagesPerProduct))
	require.Equal(t, 50, plangate.Limit(subscription.PlanStudio, plangate.FeatureImagesPerProduct))
	require.Equal(t, plangate.Unlimited, plangate.Limit(subscription.PlanPro, plangate.FeatureImagesPerProduct))
}

func TestMatrix_CampaignEmailsPerMonth(t *testing.T) {
	require.Equal(t, 5_000, plangate.Limit(subscription.PlanTrial, plangate.FeatureCampaignEmailsPerMonth))
	require.Equal(t, 15_000, plangate.Limit(subscription.PlanStarter, plangate.FeatureCampaignEmailsPerMonth))
	require.Equal(t, 50_000, plangate.Limit(subscription.PlanStudio, plangate.FeatureCampaignEmailsPerMonth))
	require.Equal(t, plangate.Negotiated, plangate.Limit(subscription.PlanPro, plangate.FeatureCampaignEmailsPerMonth))
}

func TestMatrix_CustomCSS_StudioAndUp(t *testing.T) {
	require.False(t, plangate.IsAllowed(subscription.PlanTrial, plangate.FeatureCustomCSS))
	require.False(t, plangate.IsAllowed(subscription.PlanStarter, plangate.FeatureCustomCSS))
	require.True(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureCustomCSS))
	require.True(t, plangate.IsAllowed(subscription.PlanPro, plangate.FeatureCustomCSS))
}

func TestMatrix_CustomCodeInjection_ProOnly(t *testing.T) {
	require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureCustomCodeInjection))
	require.True(t, plangate.IsAllowed(subscription.PlanPro, plangate.FeatureCustomCodeInjection))
}

func TestMatrix_SSO_ProOnly(t *testing.T) {
	require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureSSO))
	require.True(t, plangate.IsAllowed(subscription.PlanPro, plangate.FeatureSSO))
}

func TestMatrix_FullAPI_ProOnly(t *testing.T) {
	require.True(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureReadAPI))
	require.False(t, plangate.IsAllowed(subscription.PlanStudio, plangate.FeatureFullAPI))
	require.True(t, plangate.IsAllowed(subscription.PlanPro, plangate.FeatureFullAPI))
}

func TestMatrix_WhiteLabelApp_GatedByAddOn(t *testing.T) {
	for _, p := range []subscription.SubscriptionPlan{
		subscription.PlanTrial, subscription.PlanStarter, subscription.PlanStudio, subscription.PlanPro,
	} {
		require.False(t, plangate.IsAllowed(p, plangate.FeatureWhiteLabelApp),
			"FeatureWhiteLabelApp must be Disabled across all plans; gate via WhiteLabelAppEnabled")
	}
	require.True(t, plangate.WhiteLabelAppEnabled(subscription.PlanPro, true))
	require.False(t, plangate.WhiteLabelAppEnabled(subscription.PlanPro, false))
	require.False(t, plangate.WhiteLabelAppEnabled(subscription.PlanStudio, true))
}

func TestMatrix_AuditRetention(t *testing.T) {
	require.Equal(t, 90, plangate.Limit(subscription.PlanTrial, plangate.FeatureAuditRetentionDays))
	require.Equal(t, 90, plangate.Limit(subscription.PlanStarter, plangate.FeatureAuditRetentionDays))
	require.Equal(t, 365, plangate.Limit(subscription.PlanStudio, plangate.FeatureAuditRetentionDays))
	require.Equal(t, plangate.Unlimited, plangate.Limit(subscription.PlanPro, plangate.FeatureAuditRetentionDays))
}

// TestAllFeatureLimits_EveryFeaturePresentForEveryPlan guards the contract
// that AllFeatureLimits emits one key per Feature, for every customer-
// facing plan. Frontends iterate AllFeatures() and look up values — a
// missing key would surface as a silent "disabled" render.
func TestAllFeatureLimits_EveryFeaturePresentForEveryPlan(t *testing.T) {
	for _, p := range []subscription.SubscriptionPlan{
		subscription.PlanTrial,
		subscription.PlanStarter,
		subscription.PlanStudio,
		subscription.PlanPro,
	} {
		t.Run(string(p), func(t *testing.T) {
			m := plangate.AllFeatureLimits(p)
			for _, f := range plangate.AllFeatures() {
				_, ok := m[string(f)]
				require.True(t, ok, "feature %s missing from AllFeatureLimits for %s", f, p)
			}
		})
	}
}

// TestAllFeatures_IncludesAllConstants sanity-checks that the canonical
// feature list stays aligned with the spec §9 count. Bumping the count
// should be deliberate — update this assertion when a new Feature lands.
func TestAllFeatures_IncludesAllConstants(t *testing.T) {
	require.Equal(t, 25, len(plangate.AllFeatures()),
		"expected 25 feature constants per §9 — update this count if new ones are added")
}
