// Command genpricing prints packages/ui/src/subscription/pricing-data.ts,
// generated from internal/billing/pricing's catalog (developedAmounts,
// pppAmounts) plus its display_extras.go declarations. Run it and redirect
// stdout over the committed file:
//
//	cd services/marketplace-api && go run ./internal/billing/pricing/cmd/genpricing > \
//	  ../../packages/ui/src/subscription/pricing-data.ts
//
// See pricing.RegenerateCommand for the single source of that command
// string (also quoted in the generated file's header and in
// genpricing_test.go's failure message).
package main

import (
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
)

func main() {
	out, err := pricing.GenerateTS()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genpricing:", err) //nolint:logging-smoke // dev-only CLI: human-readable error to stderr before exit
		os.Exit(1)
	}
	os.Stdout.WriteString(out)
}
