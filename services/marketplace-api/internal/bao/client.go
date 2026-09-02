// Package bao is marketplace-api's client for OpenBao — the secret store
// that per-tenant payment/shipping/tax credentials are migrating to from GCP
// Secret Manager. It is ported from secret-service's internal/bao client
// (github.com/tesserix/secret-service/apps/api/internal/bao), which already
// gets the hard parts right: a mutex-guarded lazy re-login that renews the
// OpenBao token before it expires, and a Health check that distinguishes
// "unreachable" from "not yet unsealed".
package bao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	openbao "github.com/openbao/openbao/api/v2"
)

// DefaultServiceAccountTokenPath is where the kubelet projects the pod's token.
const DefaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// ErrNotFound is returned when OpenBao reports a 404 — the secret (or the
// path) does not exist.
var ErrNotFound = errors.New("bao: not found")

// ErrForbidden is returned when OpenBao reports a 403. marketplace-api's
// role is deliberately scoped KV-only, so callers must be able to tell this
// apart from a transient failure — some code paths (e.g. a storefront role
// attempting a write it isn't granted) expect to hit this.
var ErrForbidden = errors.New("bao: forbidden")

// ErrConflict is returned when a KV v2 check-and-set write loses a race
// against a concurrent write to the same secret.
var ErrConflict = errors.New("bao: version conflict")

// Config configures a Client's connection to OpenBao and how it authenticates.
type Config struct {
	Address string
	Mount   string

	// Token short-circuits Kubernetes auth. Local development only.
	Token string

	KubernetesRole      string
	KubernetesMount     string
	ServiceAccountToken string
}

// Client is marketplace-api's only route to OpenBao. It holds no long-lived
// credential: in-cluster it exchanges the pod's projected ServiceAccount token
// for a short-TTL OpenBao token and renews it before expiry.
type Client struct {
	api   *openbao.Client
	mount string
	cfg   Config

	mu          sync.Mutex
	tokenExpiry time.Time
}

// New builds a Client. It does not authenticate — that happens lazily on
// first use.
func New(cfg Config) (*Client, error) {
	if cfg.Address == "" {
		return nil, errors.New("bao: address is required")
	}
	if cfg.Mount == "" {
		cfg.Mount = "kv"
	}
	if cfg.KubernetesMount == "" {
		cfg.KubernetesMount = "kubernetes"
	}
	if cfg.ServiceAccountToken == "" {
		cfg.ServiceAccountToken = DefaultServiceAccountTokenPath
	}

	apiCfg := openbao.DefaultConfig()
	apiCfg.Address = cfg.Address
	apiCfg.MaxRetries = 2
	apiCfg.Timeout = 15 * time.Second

	client, err := openbao.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("bao: new client: %w", err)
	}
	if cfg.Token != "" {
		client.SetToken(cfg.Token)
	}

	return &Client{api: client, mount: cfg.Mount, cfg: cfg}, nil
}

// authenticate exchanges the pod's ServiceAccount token for an OpenBao token,
// renewing when the current one is within a minute of expiry.
func (c *Client) authenticate(ctx context.Context) error {
	if c.cfg.Token != "" {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Now().Add(time.Minute).Before(c.tokenExpiry) {
		return nil
	}

	jwt, err := os.ReadFile(c.cfg.ServiceAccountToken)
	if err != nil {
		return fmt.Errorf("bao: read service account token: %w", err)
	}

	path := fmt.Sprintf("auth/%s/login", c.cfg.KubernetesMount)
	resp, err := c.api.Logical().WriteWithContext(ctx, path, map[string]any{
		"role": c.cfg.KubernetesRole,
		"jwt":  strings.TrimSpace(string(jwt)),
	})
	if err != nil {
		return fmt.Errorf("bao: kubernetes login: %w", translate(err))
	}
	if resp == nil || resp.Auth == nil {
		return errors.New("bao: kubernetes login returned no auth block")
	}

	c.api.SetToken(resp.Auth.ClientToken)
	c.tokenExpiry = time.Now().Add(time.Duration(resp.Auth.LeaseDuration) * time.Second)
	return nil
}

// Health reports whether OpenBao is reachable, initialised and unsealed.
func (c *Client) Health(ctx context.Context) error {
	health, err := c.api.Sys().HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("bao: health: %w", translate(err))
	}
	if !health.Initialized || health.Sealed {
		return fmt.Errorf("bao: not ready (initialized=%t sealed=%t)", health.Initialized, health.Sealed)
	}
	return nil
}

// translate maps OpenBao's HTTP-status errors onto this package's sentinels
// so callers can use errors.Is instead of inspecting status codes.
func translate(err error) error {
	var respErr *openbao.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.StatusCode {
		case 404:
			return ErrNotFound
		case 403:
			return ErrForbidden
		case 400:
			// KV v2 reports a lost check-and-set race as a plain 400; the text is
			// the only thing distinguishing it from a malformed write.
			if strings.Contains(respErr.Error(), "check-and-set parameter did not match") {
				return ErrConflict
			}
		}
	}
	return err
}
