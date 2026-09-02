package bao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// cleanSecretPath is a minimal stand-in for secret-service's
// secrets.CleanSecretPath — this package does not import secret-service.
// It trims slashes and rejects the empty and traversal cases; callers are
// internal (a later task's KV backend), not raw user input, so it does not
// need to be exhaustive.
func cleanSecretPath(path string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return "", errors.New("bao: secret path must not be empty")
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("bao: invalid secret path segment %q", seg)
		}
	}
	return trimmed, nil
}

// ReadSecret reads the current version of a KV v2 secret's data from the
// DATA path.
func (c *Client) ReadSecret(ctx context.Context, path string) (map[string]string, int, error) {
	clean, err := cleanSecretPath(path)
	if err != nil {
		return nil, 0, err
	}
	if err := c.authenticate(ctx); err != nil {
		return nil, 0, err
	}

	resp, err := c.api.Logical().ReadWithContext(ctx, c.dataPath(clean))
	if err != nil {
		return nil, 0, translate(err)
	}
	if resp == nil || resp.Data == nil {
		return nil, 0, ErrNotFound
	}

	raw, _ := resp.Data["data"].(map[string]any)
	if raw == nil {
		return nil, 0, ErrNotFound
	}
	data := make(map[string]string, len(raw))
	for k, v := range raw {
		s, ok := v.(string)
		if !ok {
			continue
		}
		data[k] = s
	}

	version := 0
	if meta, ok := resp.Data["metadata"].(map[string]any); ok {
		version = intFrom(meta["version"])
	}
	return data, version, nil
}

// WriteSecret creates a new version of a KV v2 secret at the DATA path. It is
// deliberately blind: the caller supplies the whole map because nothing here
// can read the old one.
//
// ifVersion is the version the caller believes is current; a positive value
// travels as KV v2's check-and-set, so a write drawn from a stale read is
// refused (ErrConflict) rather than silently overwriting someone else's.
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]string, ifVersion int) (int, error) {
	clean, err := cleanSecretPath(path)
	if err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, errors.New("bao: refusing to write a secret with no keys")
	}

	payload := make(map[string]any, len(data))
	for k, v := range data {
		if strings.TrimSpace(k) == "" {
			return 0, errors.New("bao: secret keys may not be blank")
		}
		payload[k] = v
	}

	if err := c.authenticate(ctx); err != nil {
		return 0, err
	}

	body := map[string]any{"data": payload}
	if ifVersion > 0 {
		body["options"] = map[string]any{"cas": ifVersion}
	}

	resp, err := c.api.Logical().WriteWithContext(ctx, c.dataPath(clean), body)
	if err != nil {
		return 0, translate(err)
	}
	if resp == nil || resp.Data == nil {
		return 0, nil
	}
	return intFrom(resp.Data["version"]), nil
}

// DestroySecret removes every version of a secret and its metadata via the
// METADATA path. This is irreversible — unlike a soft delete (which targets
// the DATA path and is restorable), there is no undo. A later task's
// DeleteSecret maps onto this, not onto a soft delete.
func (c *Client) DestroySecret(ctx context.Context, path string) error {
	clean, err := cleanSecretPath(path)
	if err != nil {
		return err
	}
	if err := c.authenticate(ctx); err != nil {
		return err
	}
	if _, err := c.api.Logical().DeleteWithContext(ctx, c.metadataPath(clean)); err != nil {
		return translate(err)
	}
	return nil
}

func (c *Client) dataPath(clean string) string { return c.mount + "/data/" + clean }

// An empty clean path addresses the mount root, which takes no trailing slash.
func (c *Client) metadataPath(clean string) string {
	return strings.TrimSuffix(c.mount+"/metadata/"+clean, "/")
}

func intFrom(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err != nil {
			return 0
		}
		return i
	}
	return 0
}
