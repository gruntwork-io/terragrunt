// Command synctestcheck is the standalone driver for the synctestcheck
// analyzer. Build it with `go install ./internal/synctestcheck/cmd/synctestcheck`
// (or run via `go run`) and invoke as a normal go vet tool.
package main

import (
	"github.com/gruntwork-io/terragrunt/internal/synctestcheck"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(synctestcheck.Analyzer)
}
