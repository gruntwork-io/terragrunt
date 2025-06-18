package runbase

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
)

// Stack represents a stack of units that you can "spin up" or "spin down"
type Stack struct {
	Report                *report.Report
	TerragruntOptions     *options.TerragruntOptions
	ChildTerragruntConfig *config.TerragruntConfig
	Units                 Units
	ParserOptions         []hclparse.Option
}

// String renders this stack as a human-readable string
func (stack *Stack) String() string {
	modules := make([]string, 0, len(stack.Units))
	for _, module := range stack.Units {
		modules = append(modules, "  => "+module.String())
	}

	sort.Strings(modules)

	return fmt.Sprintf("Stack at %s:\n%s", stack.TerragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}
