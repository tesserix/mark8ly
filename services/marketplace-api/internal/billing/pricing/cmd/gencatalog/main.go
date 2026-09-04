// Command gencatalog writes internal/billing/pricing/catalog_data.go, the
// generated half of the price catalog: the developedAmounts and pppAmounts
// tables catalog.go builds its PriceDescriptors from. Run it with -o pointing
// at the committed file:
//
//	cd services/marketplace-api && go run ./internal/billing/pricing/cmd/gencatalog \
//	  -o internal/billing/pricing/catalog_data.go
//
// -o exists rather than a stdout redirect because the destination is a file in
// the package being compiled: `> catalog_data.go` truncates it before `go run`
// builds, and the build then fails on the file it was about to replace.
//
// See pricing.RegenerateCatalogCommand for the single source of that command
// string (also quoted in the generated file's header and in
// gencatalog_test.go's failure message).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

func main() {
	out := flag.String("o", "", "write the generated file here; empty writes to stdout")
	flag.Parse()

	data, err := pricing.GenerateCatalogData()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gencatalog:", err) //nolint:logging-smoke // dev-only CLI: human-readable error to stderr before exit
		os.Exit(1)
	}

	if *out == "" {
		os.Stdout.WriteString(data)
		return
	}
	if err := os.WriteFile(*out, []byte(data), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gencatalog:", err) //nolint:logging-smoke // dev-only CLI: human-readable error to stderr before exit
		os.Exit(1)
	}
}
