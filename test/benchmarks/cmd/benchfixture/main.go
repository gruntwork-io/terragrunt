// Command benchfixture writes a benchmark fixture to disk, so that tools outside the Go benchmarks
// measure Terragrunt against the same layouts.
//
// Usage: benchfixture -dir <path> -shape <flat|deps> -units <count>
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/test/benchmarks/fixture"
)

func main() {
	dir := flag.String("dir", "", "directory to write the fixture to")
	shape := flag.String("shape", string(fixture.ShapeFlat), "unit layout: flat or deps")
	units := flag.Int("units", 1, "number of units")

	flag.Parse()

	if *dir == "" {
		fmt.Fprintln(os.Stderr, "benchfixture: -dir is required")
		os.Exit(1)
	}

	if err := fixture.Write(*dir, fixture.Shape(*shape), *units); err != nil {
		fmt.Fprintln(os.Stderr, "benchfixture:", err)
		os.Exit(1)
	}
}
