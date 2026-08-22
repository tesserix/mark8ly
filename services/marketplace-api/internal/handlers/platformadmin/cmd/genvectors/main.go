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
	// RequestTarget is the raw wire path+query — shown for illustration
	// only. It is NOT what gets signed.
	RequestTarget string `json:"request_target"`
	// Path is the decoded net/http Request.URL.Path — this is what is
	// actually signed. See the package doc on signature.go.
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
		name          string
		requestTarget string
		in            platformadmin.SignatureInput
	}{
		{
			name:          "get-with-query",
			requestTarget: "/api/v1/admin/audit-logs?since_hours=720&limit=200",
			in: platformadmin.SignatureInput{
				Method: "GET", Path: "/api/v1/admin/audit-logs",
				RawQuery:  "since_hours=720&limit=200",
				Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000001",
				Operator: "op_7f3a", Capability: "audit.read",
			},
		},
		{
			name:          "post-with-body",
			requestTarget: "/api/v1/admin/tenants/t1/suspend",
			in: platformadmin.SignatureInput{
				Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
				Body:      []byte(`{"reason_code":"fraud"}`),
				Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000002",
				Operator: "op_7f3a", Capability: "tenant.suspend",
			},
		},
		// Pins two rules most likely to be implemented wrong by a
		// reimplementation on a different stack:
		//   - repeated query keys sort their values too, not just their keys
		//     (a=z&a=a&b=2 -> a=a&a=z&b=2)
		//   - Path is signed as the DECODED net/http Request.URL.Path (a real
		//     space here), never the raw wire form ("t%20one", shown in
		//     request_target only for illustration). Signing the wire form
		//     instead of the decoded form is a permanent 401 on every
		//     percent-encoded path. See the package doc on signature.go.
		{
			name:          "repeated-query-and-encoded-path",
			requestTarget: "/api/v1/admin/tenants/t%20one?a=z&a=a&b=2",
			in: platformadmin.SignatureInput{
				Method: "GET", Path: "/api/v1/admin/tenants/t one",
				RawQuery:  "a=z&a=a&b=2",
				Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000003",
				Operator: "op_7f3a", Capability: "tenant.read",
			},
		},
		// Pins that query values are escaped with
		// application/x-www-form-urlencoded semantics (a space becomes "+"),
		// not encodeURIComponent semantics (a space becomes "%20"). A
		// reimplementation using encodeURIComponent-style escaping must
		// convert "%20" to "+" or every query value containing a space
		// silently 401s. See the package doc on signature.go.
		{
			name:          "query-value-with-space",
			requestTarget: "/api/v1/admin/audit-logs?actor=Jane%20Smith&action=product.deleted",
			in: platformadmin.SignatureInput{
				Method: "GET", Path: "/api/v1/admin/audit-logs",
				RawQuery:  "actor=Jane%20Smith&action=product.deleted",
				Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000004",
				Operator: "op_7f3a", Capability: "audit.read",
			},
		},
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
			Name: item.name, Secret: secret, RequestTarget: item.requestTarget,
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
