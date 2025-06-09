package configstack

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) *DefaultStack {
	modules := ConvertDiscoveredToModules(discovered, terragruntOptions, l)
	return &DefaultStack{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
		modules:           modules,
	}
}

// ConvertDiscoveredToModules converts discovered configs to TerraformModules (basic placeholder logic).
func ConvertDiscoveredToModules(discovered discovery.DiscoveredConfigs, terragruntOptions *options.TerragruntOptions, l log.Logger) TerraformModules {
	modules := TerraformModules{}
	for _, d := range discovered {
		if d.Parsed == nil {
			continue
		}
		mod := &TerraformModule{

			TerragruntOptions: terragruntOptions,
			Logger:            l,
			Path:              d.Path,
			Config:            *d.Parsed,
		}
		modules = append(modules, mod)
	}
	return modules
}
