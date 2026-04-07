// Command seed loads seed data from JSON files into platform-api's database.
//
// Seeds are intentionally separate from migrations: they have different
// lifecycles, change per environment, and need to be re-runnable.
//
// Phase A is a no-op stub. Phase C wires real seed data (countries/states/cities).
package main

import (
	"fmt"

	"github.com/mark8ly/platform-api/pkg/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	_ = cfg
	fmt.Println("seed: nothing to do (Phase A stub)")
}
