package arbitrage

import (
	"context"
	"fmt"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

// SecretManagerSource is the production VersionsSource backed by GCP Secret Manager.
// It implements ListEnabled against the secretmanager.Client and returns every
// enabled version with its payload and creation time.
//
// The adapter is intentionally thin: no caching (KeyLoader handles that) and
// the age filter is deferred to callers (every enabled version is returned;
// the rotation job applies its own 61-day cutoff).
type SecretManagerSource struct {
	Client     *secretmanager.Client
	SecretPath string // "projects/tesserix-prod/secrets/arbitrage-ip-hmac-key"
}

// ListEnabled iterates all enabled Secret Manager versions, fetches each
// payload, and returns them as KeyVersion slices (newest-first sorting
// deferred to KeyLoader).
func (s *SecretManagerSource) ListEnabled(ctx context.Context) ([]KeyVersion, error) {
	it := s.Client.ListSecretVersions(ctx, &smpb.ListSecretVersionsRequest{
		Parent: s.SecretPath,
		Filter: "state:ENABLED",
	})

	var versions []KeyVersion
	for {
		v, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("secretmanager: list versions for %s: %w", s.SecretPath, err)
		}

		resp, err := s.Client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{
			Name: v.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("secretmanager: access version %s: %w", v.Name, err)
		}

		var createdAt time.Time
		if v.CreateTime != nil {
			createdAt = v.CreateTime.AsTime()
		}

		versions = append(versions, KeyVersion{
			Name:      v.Name,
			Payload:   resp.Payload.Data,
			CreatedAt: createdAt,
		})
	}
	return versions, nil
}
