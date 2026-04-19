package sso

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AttrMapping is a minimal JSON-path subset for mapping IdP claims to
// Mark8ly user fields. The admin UI writes rows like:
//
//	{"email":"claims.email", "firstName":"claims.given_name", "role":"claims.groups[0]"}
//
// Every RHS MUST begin with `claims.` — the DSL is deliberately small
// so an arbitrary user-supplied path can't walk outside the claims bag
// on the server side. Missing paths are skipped silently at Resolve
// time; the JIT provisioner decides which resolved attrs are required.
type AttrMapping map[string]string

// segmentRE matches `name` or `name[N]`. Digits-only segments without a
// leading identifier are rejected by Validate.
var segmentRE = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)(?:\[(\d+)\])?$`)

// Validate enforces the `claims.` root and legal segment syntax.
// Called by the admin Upsert handler before any DB write.
func (m AttrMapping) Validate() error {
	for k, v := range m {
		if k == "" {
			return fmt.Errorf("%w: empty key", ErrInvalidAttrMap)
		}
		if !strings.HasPrefix(v, "claims.") {
			return fmt.Errorf("%w: %q must start with 'claims.'", ErrInvalidAttrMap, k)
		}
		rest := strings.TrimPrefix(v, "claims.")
		if rest == "" {
			return fmt.Errorf("%w: %q has no path after 'claims.'", ErrInvalidAttrMap, k)
		}
		for _, seg := range strings.Split(rest, ".") {
			if !segmentRE.MatchString(seg) {
				return fmt.Errorf("%w: %q has invalid segment %q", ErrInvalidAttrMap, k, seg)
			}
		}
	}
	return nil
}

// Resolve walks each mapping RHS against root and returns the resolved
// scalar values keyed by the mapping's LHS. Paths that point at missing
// or wrong-typed data are silently omitted from the result — this is
// intentional so an IdP that's missing, say, `given_name` doesn't
// break login; the JIT layer gets to decide what's required.
//
// `root` is expected to have a top-level "claims" key whose value is a
// map[string]any of the IdP's raw claims/attributes.
func (m AttrMapping) Resolve(root map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(m))
	for k, v := range m {
		val, ok := walk(root, v)
		if !ok {
			continue
		}
		out[k] = val
	}
	return out, nil
}

// walk traverses `root` using a simple path language: dot-separated
// segments, each optionally followed by `[N]` for array indexing.
// Returns (value, true) on a full match; (nil, false) on any dead-end.
func walk(root map[string]any, path string) (any, bool) {
	if !strings.HasPrefix(path, "claims.") {
		return nil, false
	}
	rest := strings.TrimPrefix(path, "claims.")

	var cur any = root["claims"]
	for _, seg := range strings.Split(rest, ".") {
		match := segmentRE.FindStringSubmatch(seg)
		if match == nil {
			return nil, false
		}
		name, idx := match[1], match[2]

		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = obj[name]
		if !ok {
			return nil, false
		}

		if idx != "" {
			n, err := strconv.Atoi(idx)
			if err != nil {
				return nil, false
			}
			arr, ok := cur.([]any)
			if !ok || n < 0 || n >= len(arr) {
				return nil, false
			}
			cur = arr[n]
		}
	}
	return cur, true
}
