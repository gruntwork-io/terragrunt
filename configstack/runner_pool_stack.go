package configstack

import (
	"context"
	"sync"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// RunnerPoolStack implements the Stack interface for runner pool execution.
type RunnerPoolStack struct {
	modules           TerraformModules
	report            *report.Report
	parserOptions     []hclparse.Option
	terragruntOptions *options.TerragruntOptions
	outputMu          sync.Mutex
	childConfig       *config.TerragruntConfig
}

// NewRunnerPoolStack creates a new stack from discovered modules.
func NewRunnerPoolStack(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, discovered discovery.DiscoveredConfigs) (*DefaultStack, error) {
	modulesMap := make(TerraformModulesMap)

	stack := &DefaultStack{
		terragruntOptions: terragruntOptions,
		parserOptions:     config.DefaultParserOptions(l, terragruntOptions),
	}

	var terragruntConfigPaths []string
	for _, cfg := range discovered {
		configPath := config.GetDefaultConfigPath(cfg.Path)
		if cfg.Parsed == nil {
			// Skip configurations that could not be parsed
			l.Warnf("Skipping module at %s due to parse error: %s", cfg.Path, configPath)
			continue
		}
		modLogger, modOpts, err := terragruntOptions.CloneWithConfigPath(l, configPath)
		if err != nil {
			l.Warnf("Skipping module at %s due to error cloning options: %s", cfg.Path, err)
			continue // skip on error
		}
		mod := &TerraformModule{
			Stack:             stack,
			TerragruntOptions: modOpts,
			Logger:            modLogger,
			Path:              cfg.Path,
			Config:            *cfg.Parsed,
		}
		terragruntConfigPaths = append(terragruntConfigPaths, configPath)
		modulesMap[cfg.Path] = mod
	}

	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	linkedModules, err := modulesMap.crosslinkDependencies(canonicalTerragruntConfigPaths)
	if err != nil {
		return nil, err
	}

	stack.modules = linkedModules

	return stack, nil
}
