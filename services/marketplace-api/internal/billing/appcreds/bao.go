package appcreds

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mark8ly/marketplace-api/internal/bao"
)

type baoSM struct {
	client    *bao.Client
	projectID string
}

// NewBaoSM stores mobile-app credentials under the authenticated tenant's OpenBao subtree.
func NewBaoSM(client *bao.Client, projectID string) SM {
	return &baoSM{client: client, projectID: projectID}
}

func (b *baoSM) path(name string) (string, error) {
	id, _, err := splitSecretName(name, b.projectID)
	if err != nil {
		return "", errors.New("appcreds: invalid credential name")
	}
	parts := strings.Split(id, "_")
	if len(parts) != 3 || parts[0] != "merchant" {
		return "", errors.New("appcreds: invalid credential name")
	}
	tenant, err := uuid.Parse(parts[1])
	if err != nil || tenant.String() != parts[1] {
		return "", errors.New("appcreds: invalid tenant")
	}
	if err := validateCommonInput(tenant, CredType(parts[2])); err != nil {
		return "", err
	}
	return "mark8ly/marketplace-api/tenants/" + tenant.String() + "/app-credentials/mark8ly-" + parts[2], nil
}

func (b *baoSM) CreateOrAddVersion(ctx context.Context, name string, payload []byte) error {
	path, err := b.path(name)
	if err != nil {
		return err
	}
	_, err = b.client.WriteSecret(ctx, path, map[string]string{"value": base64.StdEncoding.EncodeToString(payload)}, 0)
	return err
}

func (b *baoSM) AccessLatest(ctx context.Context, name string) ([]byte, error) {
	path, err := b.path(name)
	if err != nil {
		return nil, err
	}
	data, _, err := b.client.ReadSecret(ctx, path)
	if errors.Is(err, bao.ErrNotFound) {
		return nil, ErrSMNotFound
	}
	if err != nil {
		return nil, err
	}
	value, ok := data["value"]
	if !ok {
		return nil, errors.New("appcreds: stored credential has no value")
	}
	payload, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("appcreds: invalid credential encoding: %w", err)
	}
	return payload, nil
}

func (b *baoSM) Delete(ctx context.Context, name string) error {
	path, err := b.path(name)
	if err != nil {
		return err
	}
	err = b.client.DestroySecret(ctx, path)
	if errors.Is(err, bao.ErrNotFound) {
		return nil
	}
	return err
}
