package validate

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// summaryDeclaredOutputNotInModule heads a diagnostic for an output the unit declares and its module does not.
	summaryDeclaredOutputNotInModule = "Declared output not in module"
	// summaryUndeclaredDependencyOutput heads a diagnostic for a reference to an output a dependency does not declare.
	summaryUndeclaredDependencyOutput = "Undeclared dependency output"
)

// validateOutputsContracts checks the outputs contracts of the unit at opts.TerragruntConfigPath under the
// outputs-block experiment: every output its outputs block declares exists in its OpenTofu/Terraform module, and
// every dependency output it reads is declared by the dependency, when that dependency declares outputs. It
// collects a diagnostic per failed check and does nothing when the experiment is off.
func (p ComponentParser) validateOutputsContracts(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
) error {
	if !opts.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil
	}

	if cfg.Outputs == nil && len(cfg.TerragruntDependencies) == 0 {
		return nil
	}

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, opts.TerragruntConfigPath)
	if err != nil {
		return err
	}

	var diags hcl.Diagnostics

	if cfg.Outputs != nil {
		moduleDiags, err := declaredOutputsDiagnostics(ctx, l, v, opts, cfg, file)
		if err != nil {
			return err
		}

		diags = append(diags, moduleDiags...)
	}

	referenceDiags, err := dependencyOutputReferenceDiagnostics(ctx, l, v, pctx, cfg, file)
	if err != nil {
		return err
	}

	return p.Collector.keep(file.File, append(diags, referenceDiags...))
}

// declaredOutputsDiagnostics reports each output the unit declares that its OpenTofu/Terraform module does not.
func declaredOutputsDiagnostics(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	file *hclparse.File,
) (hcl.Diagnostics, error) {
	moduleDir, err := moduleDirectory(ctx, l, v, opts, cfg)
	if err != nil {
		return nil, err
	}

	moduleOutputs, err := tf.ModuleOutputs(v.FS, moduleDir)
	if err != nil {
		return nil, err
	}

	ranges := config.OutputDeclarationRanges(file)

	var diags hcl.Diagnostics

	for _, name := range cfg.Outputs.Names() {
		if slices.Contains(moduleOutputs, name) {
			continue
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryDeclaredOutputNotInModule,
			Detail: fmt.Sprintf(
				"The outputs block declares output %q, but the OpenTofu/Terraform module at %s has no output %q. "+
					"Remove the declaration or add the output to the module.",
				name,
				moduleDir,
				name,
			),
			Subject: declarationSubject(file, ranges, name),
		})
	}

	return diags, nil
}

// moduleDirectory returns the directory of the OpenTofu/Terraform module the unit runs, prepared the way a run
// prepares it: a unit with a source is fetched into the download directory, and the generate blocks are written.
// A unit without a source runs in its own directory.
func moduleDirectory(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (string, error) {
	moduleOpts := opts

	hasSource := cfg.Terraform != nil && cfg.Terraform.Source != nil && *cfg.Terraform.Source != ""
	if hasSource || opts.Source != "" {
		prepared, err := prepare.PrepareSource(ctx, l, v, opts, cfg, report.NewReport())
		if err != nil {
			return "", err
		}

		moduleOpts = prepared
	}

	if err := prepare.PrepareGenerate(ctx, l, v, moduleOpts, cfg.ToRunConfig(l, v.FS)); err != nil {
		return "", err
	}

	return moduleOpts.WorkingDir, nil
}

// declarationSubject returns where the declaration of name is, or the start of the file for a config whose
// declarations have no range, such as a JSON config.
func declarationSubject(file *hclparse.File, ranges map[string]hcl.Range, name string) *hcl.Range {
	if rng, ok := ranges[name]; ok {
		return &rng
	}

	rng := file.Body.MissingItemRange()

	return &rng
}

// dependencyOutputReferenceDiagnostics reports each reference in file to an output that the dependency it reads
// does not declare. A dependency that declares no outputs block is not checked.
func dependencyOutputReferenceDiagnostics(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	file *hclparse.File,
) (hcl.Diagnostics, error) {
	references, err := config.DependencyOutputReferences(file)
	if err != nil {
		return nil, err
	}

	if len(references) == 0 {
		return nil, nil
	}

	declared := make(map[string]*config.OutputsConfig, len(cfg.TerragruntDependencies))

	var diags hcl.Diagnostics

	for _, reference := range references {
		address := reference.Address()

		contract, read := declared[address]
		if !read {
			var err error

			contract, err = declaredOutputsOf(ctx, l, v, pctx, cfg, address)
			if err != nil {
				return nil, err
			}

			declared[address] = contract
		}

		if contract == nil || contract.Declares(reference.Output) {
			continue
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryUndeclaredDependencyOutput,
			Detail: fmt.Sprintf(
				"dependency %q declares its outputs, and %q is not one of them. Declared outputs: %s. "+
					"Fix the reference or add output %q to the outputs block of the dependency.",
				address,
				reference.Output,
				strings.Join(contract.Names(), ", "),
				reference.Output,
			),
			Subject: &reference.Range,
		})
	}

	return diags, nil
}

// declaredOutputsOf returns the outputs block of the unit the dependency at address reads, or nil when there is
// no such enabled dependency with a config path that exists, or the unit declares none.
func declaredOutputsOf(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	address string,
) (*config.OutputsConfig, error) {
	for i := range cfg.TerragruntDependencies {
		dep := &cfg.TerragruntDependencies[i]
		if dep.Address() != address || !config.IsValidConfigPath(dep.ConfigPath) {
			continue
		}

		if dep.Enabled != nil && !*dep.Enabled {
			continue
		}

		targetConfig := dep.ConfigPath.AsString()
		if !filepath.IsAbs(targetConfig) {
			targetConfig = filepath.Join(filepath.Dir(pctx.TerragruntConfigPath), targetConfig)
		}

		if vfs.IsDir(v.FS, targetConfig) {
			targetConfig = config.GetDefaultConfigPath(v.FS, targetConfig)
		}

		if !vfs.Exists(v.FS, targetConfig) {
			return nil, nil
		}

		return config.ReadDeclaredOutputs(ctx, l, v, pctx, filepath.Clean(targetConfig))
	}

	return nil, nil
}
