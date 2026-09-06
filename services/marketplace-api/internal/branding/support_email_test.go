package branding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// fakeRepo stands in for Postgres so these tests can pin Service.Update's
// field-merge behaviour without a database. It records the row Update
// handed to Upsert, which is what would have been written.
type fakeRepo struct {
	existing *StoreBranding
	upserted *StoreBranding
}

func (f *fakeRepo) GetByStoreID(context.Context, *gorm.DB, uuid.UUID) (*StoreBranding, error) {
	if f.existing == nil {
		return nil, apperrors.NotFound("branding")
	}
	clone := *f.existing
	return &clone, nil
}

func (f *fakeRepo) Upsert(_ context.Context, _ *gorm.DB, b *StoreBranding) error {
	clone := *b
	f.upserted = &clone
	return nil
}

func ptr(s string) *string { return &s }

func newTestService(existing *StoreBranding) (*Service, *fakeRepo) {
	repo := &fakeRepo{existing: existing}
	return NewService(ServiceConfig{Repo: repo}), repo
}

func TestUpdate_SetsSupportEmail(t *testing.T) {
	storeID := uuid.New()
	svc, repo := newTestService(nil)

	got, err := svc.Update(context.Background(), UpdateInput{
		StoreID:      storeID,
		SupportEmail: ptr("hello@nadiasceramics.com"),
	})
	require.NoError(t, err)
	require.Equal(t, "hello@nadiasceramics.com", got.SupportEmail)
	require.Equal(t, "hello@nadiasceramics.com", repo.upserted.SupportEmail,
		"the value must reach the row that is written, not just the response")
}

func TestUpdate_TrimsSupportEmail(t *testing.T) {
	svc, repo := newTestService(nil)
	_, err := svc.Update(context.Background(), UpdateInput{
		StoreID:      uuid.New(),
		SupportEmail: ptr("  hello@nadiasceramics.com  "),
	})
	require.NoError(t, err)
	require.Equal(t, "hello@nadiasceramics.com", repo.upserted.SupportEmail)
}

func TestUpdate_ClearsSupportEmail(t *testing.T) {
	storeID := uuid.New()
	existing := defaultBranding(storeID)
	existing.SupportEmail = "hello@nadiasceramics.com"
	svc, repo := newTestService(existing)

	got, err := svc.Update(context.Background(), UpdateInput{
		StoreID:      storeID,
		SupportEmail: ptr(""),
	})
	require.NoError(t, err)
	require.Equal(t, "", got.SupportEmail)
	require.Equal(t, "", repo.upserted.SupportEmail,
		"an explicit empty string is the merchant clearing the field")
}

// TestUpdate_OmittedSupportEmailIsPreserved is the acceptance criterion
// from #749: a branding save that does not mention support_email must not
// destroy it. Upsert writes every mapped column via Select("*"), so the
// only thing standing between a colour change and a wiped contact address
// is that Update merges onto the row it just read and guards this field
// on nil.
func TestUpdate_OmittedSupportEmailIsPreserved(t *testing.T) {
	storeID := uuid.New()
	existing := defaultBranding(storeID)
	existing.SupportEmail = "hello@nadiasceramics.com"
	svc, repo := newTestService(existing)

	got, err := svc.Update(context.Background(), UpdateInput{
		StoreID:     storeID,
		ColorAccent: ptr("#2D4A2B"),
		// SupportEmail deliberately omitted.
	})
	require.NoError(t, err)
	require.Equal(t, "hello@nadiasceramics.com", got.SupportEmail)
	require.Equal(t, "hello@nadiasceramics.com", repo.upserted.SupportEmail,
		"an unrelated branding save must not blank the support email")
}

func TestUpdate_RejectsUnusableSupportEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"no at sign", "not-an-email"},
		{"no domain dot", "nadia@localhost"},
		{"display name form", "Nadia <nadia@nadiasceramics.com>"},
		{"angle brackets", "<nadia@nadiasceramics.com>"},
		{"header injection", "nadia@nadiasceramics.com\nBcc: attacker@evil.com"},
		{"trailing comment", "nadia@nadiasceramics.com (Nadia)"},
		{"address list", "a@nadiasceramics.com, b@nadiasceramics.com"},
		{"unroutable placeholder tld", "nadia@nadiasceramics.local"},
		{"reserved example tld", "nadia@nadiasceramics.example"},
		{"over column width", strings.Repeat("a", 250) + "@nadiasceramics.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := newTestService(nil)
			_, err := svc.Update(context.Background(), UpdateInput{
				StoreID:      uuid.New(),
				SupportEmail: ptr(tc.input),
			})
			require.Error(t, err)
			require.True(t, errors.Is(err, apperrors.ErrValidationFailed),
				"want a field validation error, got %v", err)
			require.Nil(t, repo.upserted,
				"a rejected address must not reach the database")
		})
	}
}

// TestSupportEmail_SaveAndSendAgree: every address the admin API accepts
// must be one email.StoreIdentity will actually put in Reply-To. If the
// two guards ever diverge, a merchant saves an address, sees it echoed
// back, and their customers still reply to the platform.
func TestSupportEmail_SaveAndSendAgree(t *testing.T) {
	accepted := []string{
		"hello@nadiasceramics.com",
		"support+orders@nadias-ceramics.co.uk",
		"a@b.io",
	}
	for _, addr := range accepted {
		t.Run(addr, func(t *testing.T) {
			svc, _ := newTestService(nil)
			got, err := svc.Update(context.Background(), UpdateInput{
				StoreID:      uuid.New(),
				SupportEmail: ptr(addr),
			})
			require.NoError(t, err)
			require.Equal(t, addr, got.SupportEmail)
			id := email.StoreIdentity("noreply@mark8ly.com", email.StoreSender{
				Name: "Nadia's Ceramics", Slug: "nadias-ceramics",
				ContactEmail: got.SupportEmail,
			})
			require.Equal(t, addr, id.ReplyTo,
				"saved address must survive the sender's own recipient guard")
		})
	}
}
