// Command genvectors prints signature reference vectors as JSON. Run it and
// commit the output to testdata/vectors.json, then paste it on #275 so the
// console can verify its implementation against ours.
//
//	go run ./internal/handlers/platformadmin/cmd/genvectors > \
//	  internal/handlers/platformadmin/testdata/vectors.json
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

type vector struct {
	Name       string `json:"name"`
	Secret     string `json:"secret"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	RawQuery   string `json:"raw_query"`
	Body       string `json:"body"`
	Timestamp  string `json:"timestamp"`
	Nonce      string `json:"nonce"`
	Operator   string `json:"operator"`
	Capability string `json:"capability"`
	Canonical  string `json:"canonical"`
	Signature  string `json:"signature"`
}

func main() {
	inputs := []struct {
		name string
		in   platformadmin.SignatureInput
	}{
		{"get-with-query", platformadmin.SignatureInput{
			Method: "GET", Path: "/api/v1/admin/audit-logs",
			RawQuery:  "since_hours=720&limit=200",
			Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000001",
			Operator: "op_7f3a", Capability: "audit.read",
		}},
		{"post-with-body", platformadmin.SignatureInput{
			Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
			Body:      []byte(`{"reason_code":"fraud"}`),
			Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000002",
			Operator: "op_7f3a", Capability: "tenant.suspend",
		}},
		// Pins two rules most likely to be implemented wrong by a
		// reimplementation on a different stack:
		//   - repeated query keys sort their values too, not just their keys
		//     (a=z&a=a&b=2 -> a=a&a=z&b=2)
		//   - Path is signed exactly as given here. This mirrors the decoded
		//     net/http Request.URL.Path a later task passes in — never
		//     RawPath/EscapedPath. See the package doc on signature.go.
		{"repeated-query-and-encoded-path", platformadmin.SignatureInput{
			Method: "GET", Path: "/api/v1/admin/tenants/t%20one",
			RawQuery:  "a=z&a=a&b=2",
			Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000003",
			Operator: "op_7f3a", Capability: "tenant.read",
		}},
	}

	const secret = "reference-secret-do-not-use"
	out := make([]vector, 0, len(inputs))

	for _, item := range inputs {
		canonical, err := platformadmin.CanonicalString(item.in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "canonical:", err)
			os.Exit(1)
		}
		sig, err := platformadmin.Sign(secret, item.in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sign:", err)
			os.Exit(1)
		}
		out = append(out, vector{
			Name: item.name, Secret: secret,
			Method: item.in.Method, Path: item.in.Path, RawQuery: item.in.RawQuery,
			Body: string(item.in.Body), Timestamp: item.in.Timestamp, Nonce: item.in.Nonce,
			Operator: item.in.Operator, Capability: item.in.Capability,
			Canonical: canonical, Signature: sig,
		})
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	// This file is read by humans on the console team as a specification,
	// not just parsed. Default encoding/json HTML-escapes '&' to "&",
	// which is harmless to parsers but would surprise anyone hand-copying a
	// query string out of the document while chasing a signature mismatch.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	os.Stdout.Write(buf.Bytes())
}
