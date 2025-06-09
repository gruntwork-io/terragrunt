package configstack

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/util"
)

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) *DefaultStack {
	modules := ConvertDiscoveredToModules(discovered, terragruntOptions, l)
	stack := &DefaultStack{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
		modules:           modules,
	}
	// Set the Stack field for each module
	for _, m := range modules {
		m.Stack = stack
	}
	return stack
}

// ConvertDiscoveredToModules converts discovered configs to TerraformModules (basic placeholder logic).
func ConvertDiscoveredToModules(discovered discovery.DiscoveredConfigs, terragruntOptions *options.TerragruntOptions, l log.Logger) TerraformModules {
	modules := TerraformModules{}
	pathToModule := map[string]*TerraformModule{}

	// First pass: create modules for valid discovered configs
	for _, d := range discovered {
		if d.Parsed == nil {
			continue
		}
		configPath := filepath.Join(d.Path, config.DefaultTerragruntConfigPath)
		if !util.FileExists(configPath) {
			continue
		}
		mod := &TerraformModule{
			TerragruntOptions: terragruntOptions,
			Logger:            l,
			Path:              d.Path,
			Config:            *d.Parsed,
			Dependencies:      nil, // to be filled in next pass
		}
		modules = append(modules, mod)
		pathToModule[d.Path] = mod
	}

	// Second pass: wire up dependencies
	for _, mod := range modules {
		deps := TerraformModules{}
		if mod.Config.Dependencies != nil {
			for _, dep := range mod.Config.Dependencies.Paths {
				depPath, err := util.CanonicalPath(dep, mod.Path)
				if err == nil {
					if depMod, ok := pathToModule[depPath]; ok {
						deps = append(deps, depMod)
					}
				}
			}
		}
		mod.Dependencies = deps
	}

	return modules
}
