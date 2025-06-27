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
	units := make([]string, 0, len(stack.Units))
	for _, unit := range stack.Units {
		units = append(units, "  => "+unit.String())
	}

	sort.Strings(units)

	return fmt.Sprintf("Stack at %s:\n%s", stack.TerragruntOptions.WorkingDir, strings.Join(units, "\n"))
}

// FindUnitByPath finds a unit in the stack by its path
func (stack *Stack) FindUnitByPath(path string) *Unit {
	for _, unit := range stack.Units {
		if unit.Path == path {
			return unit
		}
	}

	return nil
}
