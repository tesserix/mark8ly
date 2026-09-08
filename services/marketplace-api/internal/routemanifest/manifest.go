// Package routemanifest defines route-manifest.json — the committed
// declaration of every frontend-facing HTTP route marketplace-api mounts —
// and the load/save/reconcile helpers its guard test uses.
//
// # Why this exists
//
// PR #826 deleted the arbitrage-appeal endpoint. apps/admin kept a page, a
// form, an API proxy, a banner, a client, hooks and Zod schemas pointing at
// it, and every gate stayed green: tsc saw a frontend that was internally
// consistent, vitest saw mocks, next build saw valid TypeScript, and the e2e
// spec passed by mocking the very endpoint that had just been deleted. The
// dangling frontend was found by grep during cleanup.
//
// Nothing in this repo could have caught that, because no artifact stated
// which routes the backend serves. This file is that artifact. The guard in
// this package's test keeps it honest against the REAL gin trees, and a
// later frontend guard asserts the paths apps/* call against it — so a
// deleted route fails the build on both sides.
//
// # Shape
//
// One document, one array of surfaces, each surface a route registrar's
// complete output:
//
//	{
//	  "surfaces": [
//	    {
//	      "name":      "admin",
//	      "registrar": "admin.RegisterAdmin",
//	      "mount":     "/api/v1",
//	      "routes":    ["DELETE /api/v1/admin/...", "GET /api/v1/admin/..."]
//	    }
//	  ]
//	}
//
// Every entry in `routes` is "METHOD PATH" with the ABSOLUTE path as gin
// reports it, `:param` placeholders included. Entries are sorted, so a diff
// of this file reads as a list of routes added and removed. The full route
// set is the union of surfaces[].routes; there is deliberately no
// second, flat copy of it to drift.
package routemanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Doc is route-manifest.json.
type Doc struct {
	// Comment is emitted first so anyone opening the file sees how to
	// regenerate it before they start hand-editing.
	Comment  string    `json:"_comment"`
	Surfaces []Surface `json:"surfaces"`
}

// Surface is one route registrar's declared output.
type Surface struct {
	// Name is the stable key a consumer looks a surface up by.
	Name string `json:"name"`
	// Registrar is the Go function that mounts these routes, for a reader
	// tracing a route back to its source.
	Registrar string `json:"registrar"`
	// Mount is the router group prefix cmd/marketplace-api/main.go passes
	// to Registrar. Informational: Routes already hold absolute paths.
	Mount string `json:"mount"`
	// Routes are "METHOD PATH" pairs, sorted.
	Routes []string `json:"routes"`
}

// Comment is the regeneration instruction written into every generated
// document. Kept here rather than in the test so the generated file and the
// failure messages cannot disagree about the command.
const Comment = "GENERATED — do not edit by hand. Regenerate with: " + UpdateCommand

// UpdateCommand regenerates route-manifest.json from the real gin trees.
const UpdateCommand = "go test ./internal/routemanifest -run TestRouteManifestMatchesMountedRoutes -update"

// Load reads and parses the manifest at path.
func Load(path string) (Doc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Doc{}, fmt.Errorf("read route manifest %s: %w", path, err)
	}
	var doc Doc
	// DisallowUnknownFields is deliberate: a typo'd or renamed key would
	// otherwise decode to an empty surface list and every assertion below
	// would pass vacuously against nothing.
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return Doc{}, fmt.Errorf("parse route manifest %s: %w", path, err)
	}
	return doc, nil
}

// Save writes doc to path as sorted, indented JSON with a trailing newline.
func Save(path string, doc Doc) error {
	doc.Comment = Comment
	for i := range doc.Surfaces {
		sort.Strings(doc.Surfaces[i].Routes)
	}
	sort.Slice(doc.Surfaces, func(i, j int) bool {
		return doc.Surfaces[i].Name < doc.Surfaces[j].Name
	})
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode route manifest: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("write route manifest %s: %w", path, err)
	}
	return nil
}

// Diff reports what the two sets disagree on. Both directions are returned
// separately and both matter: `missing` is a route the manifest declares
// that the router no longer mounts (#826 — a deletion the frontend did not
// follow), `extra` is a route the router mounts that the manifest does not
// declare (a route added without regenerating). A one-directional check
// lets the deletion pass silently, which is the whole bug.
func Diff(declared, mounted []string) (missing, extra []string) {
	inMounted := index(mounted)
	inDeclared := index(declared)
	for _, r := range declared {
		if !inMounted[r] {
			missing = append(missing, r)
		}
	}
	for _, r := range mounted {
		if !inDeclared[r] {
			extra = append(extra, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func index(routes []string) map[string]bool {
	out := make(map[string]bool, len(routes))
	for _, r := range routes {
		out[r] = true
	}
	return out
}
