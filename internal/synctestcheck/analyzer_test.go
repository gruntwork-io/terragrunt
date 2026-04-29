package synctestcheck_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/synctestcheck"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), synctestcheck.Analyzer, "a")
}
