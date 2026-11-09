package validate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	hclparser "github.com/hashicorp/hcl/v2/hclparse"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
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
	// summaryDependencyConfigUnreadable heads a diagnostic for a dependency whose unit config cannot be read.
	summaryDependencyConfigUnreadable = "Dependency config not readable"
	// summaryUnknownMockValue heads a diagnostic for a declared mock value the units that read it cannot know.
	summaryUnknownMockValue = "Unknown mock value"
)

// viewFilePerms is the mode of a module file copied into the in-memory view, which is read and never run.
const viewFilePerms = 0o644

// contractDiagnostics is the diagnostics of the outputs contract checks of one file.
type contractDiagnostics struct {
	file  *hclparse.File
	diags hcl.Diagnostics
}

// validateOutputsContracts collects a diagnostic per outputs contract the unit breaks, or [OutputsContractError].
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

	var (
		moduleFS  vfs.FS
		moduleDir string
	)

	if cfg.Outputs != nil {
		var err error

		moduleFS, moduleDir, err = moduleView(ctx, l, v, opts, cfg)
		if err != nil {
			return OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
		}
	}

	checks, err := outputsContractDiagnostics(ctx, l, v, opts, pctx, cfg, moduleFS, moduleDir)
	if err != nil {
		return err
	}

	for _, check := range checks {
		p.Collector.dropEvaluationDiagnostics(check.diags)

		if err := p.Collector.keepDistinct(check.file.File, check.diags); err != nil {
			return err
		}
	}

	return nil
}

// validateOutputsContractsPrepared returns the outputs contract diagnostics of the unit against the module at dir.
func validateOutputsContractsPrepared(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	moduleDir string,
) error {
	if !opts.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil
	}

	pctx := configbridge.NewParsingContext(opts)

	checks, err := outputsContractDiagnostics(ctx, l, v, opts, pctx, cfg, v.FS, moduleDir)
	if err != nil {
		return err
	}

	var errs []error

	for _, check := range checks {
		for _, diag := range check.diags {
			errs = append(errs, diag)
		}
	}

	return errors.Join(errs...)
}

// outputsContractDiagnostics returns the outputs contract diagnostics of the unit by file, or [OutputsContractError].
func outputsContractDiagnostics(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	moduleFS vfs.FS,
	moduleDir string,
) ([]contractDiagnostics, error) {
	if cfg.Outputs == nil && len(cfg.TerragruntDependencies) == 0 && len(cfg.ProcessedIncludes) == 0 {
		return nil, nil
	}

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, opts.TerragruntConfigPath)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	var checks []contractDiagnostics

	if cfg.Outputs != nil {
		moduleChecks, err := declaredOutputsDiagnostics(moduleFS, moduleDir, moduleEngine(opts, cfg), cfg, file)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
		}

		checks = append(checks, moduleChecks...)
	}

	replaced, err := autoIncludeReplacements(l, v, pctx, opts.TerragruntConfigPath)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	referenceDiags, err := dependencyOutputReferenceDiagnostics(ctx, l, v, pctx, cfg, file, replaced)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	checks = append(checks, contractDiagnostics{file: file, diags: referenceDiags})

	includedPaths, err := includedFiles(v.FS, opts.TerragruntConfigPath, cfg)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	for _, includedPath := range includedPaths {
		included, err := pctx.NewParser(l, v).ParseFromFile(v.FS, includedPath)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: includedPath, Err: err}
		}

		includedDiags, err := dependencyOutputReferenceDiagnostics(ctx, l, v, pctx, cfg, included, nil)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: includedPath, Err: err}
		}

		checks = append(checks, contractDiagnostics{file: included, diags: includedDiags})
	}

	return checks, nil
}

// includedFiles returns the merged or exposed include files of the unit at configPath and its autoinclude file.
func includedFiles(fsys vfs.FS, configPath string, cfg *config.TerragruntConfig) ([]string, error) {
	paths := make([]string, 0, len(cfg.ProcessedIncludes)+1)

	for _, name := range slices.Sorted(maps.Keys(cfg.ProcessedIncludes)) {
		include := cfg.ProcessedIncludes[name]

		strategy, err := include.GetMergeStrategy()
		if err != nil {
			return nil, err
		}

		if strategy == config.NoMerge && !include.GetExpose() {
			continue
		}

		paths = append(paths, include.Path)
	}

	autoInclude := autoIncludeFile(configPath)
	if vfs.Exists(fsys, autoInclude) {
		paths = append(paths, autoInclude)
	}

	return paths, nil
}

// autoIncludeFile returns the path of the autoinclude file beside the unit config at configPath.
func autoIncludeFile(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), config.DefaultAutoIncludeFile)
}

// autoIncludeReplacements returns the unexpanded dependency labels of the autoinclude file beside configPath.
func autoIncludeReplacements(
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	configPath string,
) (map[string]struct{}, error) {
	autoInclude := autoIncludeFile(configPath)
	if !vfs.Exists(v.FS, autoInclude) {
		return nil, nil
	}

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, autoInclude)
	if err != nil {
		return nil, err
	}

	return file.UnexpandedLabels(config.MetadataDependency, &config.Dependency{})
}

// moduleEngine returns the implementation of the run: terraform_binary unless --tf-path is set, else opts' own.
func moduleEngine(opts *options.TerragruntOptions, cfg *config.TerragruntConfig) tfimpl.Type {
	if !opts.TFPathExplicitlySet && cfg.TerraformBinary != "" {
		return tfimpl.FromPath(cfg.TerraformBinary)
	}

	if opts.TofuImplementation != "" && opts.TofuImplementation != tfimpl.Unknown {
		return opts.TofuImplementation
	}

	return tfimpl.FromPath(opts.TFPath)
}

// declaredOutputsDiagnostics reports each declared output the module lacks, or the module files that do not parse.
func declaredOutputsDiagnostics(
	moduleFS vfs.FS,
	moduleDir string,
	engine tfimpl.Type,
	cfg *config.TerragruntConfig,
	file *hclparse.File,
) ([]contractDiagnostics, error) {
	moduleOutputs, err := tf.ModuleOutputs(moduleFS, moduleDir, engine)
	if err != nil {
		var moduleDiags hcl.Diagnostics
		if !errors.As(err, &moduleDiags) {
			return nil, err
		}

		return moduleParseDiagnostics(moduleFS, moduleDir, engine, moduleDiags)
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

	return []contractDiagnostics{{file: file, diags: diags}}, nil
}

// moduleParseDiagnostics reports each module file engine reads that fails to parse, or moduleDiags when none does.
func moduleParseDiagnostics(
	moduleFS vfs.FS,
	moduleDir string,
	engine tfimpl.Type,
	moduleDiags hcl.Diagnostics,
) ([]contractDiagnostics, error) {
	paths, err := util.ListTfFiles(moduleFS, moduleDir)
	if err != nil {
		return nil, err
	}

	var checks []contractDiagnostics

	for _, path := range tf.EngineFiles(paths, engine) {
		src, err := vfs.ReadFile(moduleFS, path)
		if err != nil {
			return nil, err
		}

		parse := hclparser.NewParser().ParseHCL
		if strings.HasSuffix(path, ".json") {
			parse = hclparser.NewParser().ParseJSON
		}

		parsed, diags := parse(src, path)
		if !diags.HasErrors() {
			diags = diagnosticsIn(moduleDiags, path)
		}

		if len(diags) == 0 {
			continue
		}

		checks = append(checks, contractDiagnostics{
			file:  &hclparse.File{File: parsed, ConfigPath: path},
			diags: diags,
		})
	}

	if len(checks) == 0 {
		return nil, moduleDiags
	}

	return checks, nil
}

// diagnosticsIn returns the diagnostics of diags whose subject is in the file at path.
func diagnosticsIn(diags hcl.Diagnostics, path string) hcl.Diagnostics {
	var in hcl.Diagnostics

	for _, diag := range diags {
		if diag.Subject != nil && diag.Subject.Filename == path {
			in = append(in, diag)
		}
	}

	return in
}

// moduleView returns an in-memory view of the module the unit runs, generate blocks written on top, and its directory.
func moduleView(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (vfs.FS, string, error) {
	runCfg := cfg.ToRunConfig(l, v.FS)

	moduleDir, copyFilter, err := moduleSource(ctx, l, v, opts, runCfg)
	if err != nil {
		return nil, "", err
	}

	view := vfs.NewMemMapFS()

	if err := copyModuleFiles(v.FS, view, moduleDir, runCfg, copyFilter); err != nil {
		return nil, "", err
	}

	if err := generateInView(ctx, l, v.WithFS(view), moduleDir, runCfg); err != nil {
		return nil, "", err
	}

	return view, moduleDir, nil
}

// moduleSource returns the directory holding the module the unit runs and the filter of the files the run copies.
func moduleSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
) (string, util.CopyFilter, error) {
	sourceURL, err := runcfg.GetTerraformSourceURL(
		opts.Source,
		opts.SourceMap,
		opts.OriginalTerragruntConfigPath,
		runCfg,
	)
	if err != nil {
		return "", nil, err
	}

	if sourceURL != unitDirSource {
		moduleDir, err := downloadModule(ctx, l, v, opts, runCfg, sourceURL)

		return moduleDir, keepEveryFile, err
	}

	inPlace, err := runcfg.ShouldSkipCache(
		opts.Experiments.Evaluate(experiment.NoDotTerragruntCache),
		opts.Source,
		opts.TerragruntConfigPath,
		runCfg,
	)
	if err != nil {
		return "", nil, err
	}

	if inPlace {
		return opts.WorkingDir, keepEveryFile, nil
	}

	copyOpts := run.ModuleCopyOptions(configbridge.NewRunOptions(opts), runCfg)

	filter, err := util.NewCopyFilter(l, v.FS, opts.WorkingDir, copyOpts...)
	if err != nil {
		return "", nil, err
	}

	return opts.WorkingDir, filter, nil
}

// unitDirSource is the source URL of a unit whose module is its own directory.
const unitDirSource = "."

// downloadModule fetches sourceURL into the download directory of the unit, hooks left out, and returns the module dir.
func downloadModule(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	sourceURL string,
) (string, error) {
	downloadOpts := opts.Clone()

	_, defaultDownloadDir := util.DefaultWorkingAndDownloadDirs(opts.TerragruntConfigPath)
	if downloadOpts.DownloadDir == defaultDownloadDir && runCfg.DownloadDir != "" {
		downloadOpts.DownloadDir = runCfg.DownloadDir
	}

	hookless := *runCfg
	hookless.Terraform.BeforeHooks = nil
	hookless.Terraform.AfterHooks = nil
	hookless.Terraform.ErrorHooks = nil

	downloaded, err := run.DownloadTerraformSource(
		ctx,
		l,
		v,
		sourceURL,
		configbridge.NewRunOptions(downloadOpts),
		&hookless,
		report.NewReport(),
	)
	if err != nil {
		return "", err
	}

	return downloaded.CacheDir, nil
}

// keepEveryFile is the copy filter of a module the run reads as it is.
func keepEveryFile(string, bool) bool {
	return true
}

// copyModuleFiles copies the module files at dir and the existing generate targets of runCfg from source into view.
func copyModuleFiles(source, view vfs.FS, dir string, runCfg *runcfg.RunConfig, filter util.CopyFilter) error {
	paths, err := util.ListTfFiles(source, dir)
	if err != nil {
		return err
	}

	for _, target := range generateTargets(dir, runCfg) {
		info, err := source.Stat(target)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			paths = append(paths, target)
		}
	}

	if err := view.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	for _, path := range slices.Compact(slices.Sorted(slices.Values(paths))) {
		if !filter(path, false) {
			continue
		}

		data, err := vfs.ReadFile(source, path)
		if err != nil {
			return err
		}

		if err := vfs.WriteFile(view, path, data, viewFilePerms); err != nil {
			return err
		}
	}

	return nil
}

// generateTargets returns the paths the generate blocks and the remote_state block of runCfg write, resolved under dir.
func generateTargets(dir string, runCfg *runcfg.RunConfig) []string {
	targets := make([]string, 0, len(runCfg.GenerateConfigs)+1)

	for _, genCfg := range runCfg.GenerateConfigs {
		targets = append(targets, generateTarget(dir, genCfg.Path))
	}

	if runCfg.RemoteState.Config != nil && runCfg.RemoteState.Generate != nil {
		targets = append(targets, generateTarget(dir, runCfg.RemoteState.Generate.Path))
	}

	return targets
}

// generateTarget returns where a generate block with path writes when the module is at dir.
func generateTarget(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	return filepath.Join(dir, path)
}

// generateInView writes the generate blocks and the remote_state backend file of runCfg into the module at moduleDir.
func generateInView(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	moduleDir string,
	runCfg *runcfg.RunConfig,
) error {
	for _, name := range slices.Sorted(maps.Keys(runCfg.GenerateConfigs)) {
		genCfg := runCfg.GenerateConfigs[name]

		if err := codegen.WriteToFile(ctx, l, v, moduleDir, &genCfg); err != nil {
			return err
		}
	}

	if runCfg.RemoteState.Config != nil && runCfg.RemoteState.Generate != nil {
		return runCfg.RemoteState.GenerateOpenTofuCode(ctx, l, v, moduleDir)
	}

	return nil
}

// declarationSubject returns the range of the declaration of name, or the start of a file whose declarations have none.
func declarationSubject(file *hclparse.File, ranges map[string]hcl.Range, name string) *hcl.Range {
	if rng, ok := ranges[name]; ok {
		return &rng
	}

	rng := file.Body.MissingItemRange()

	return &rng
}

// dependencyOutputReferenceDiagnostics reports each reference in file to an output its dependency does not declare.
func dependencyOutputReferenceDiagnostics(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	file *hclparse.File,
	replaced map[string]struct{},
) (hcl.Diagnostics, error) {
	references, err := config.DependencyOutputReferences(file, cfg.TerragruntDependencies, replaced)
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

// declaredOutputsOf returns the outputs block of the unit the dependency at address reads, or nil when there is none.
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
		if dep.Address() != address {
			continue
		}

		return dep.DeclaredOutputs(ctx, l, v, pctx)
	}

	return nil, nil
}

// pointAtOutputsErrors turns the outputs contract errors in err into diagnostics and returns the ones it cannot place.
func (p ComponentParser) pointAtOutputsErrors(
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	configPaths []string,
	err error,
) error {
	if err == nil {
		return nil
	}

	var rest []error

	for _, leaf := range joinedErrors(err) {
		pointed, pointErr := p.pointAtOutputsError(l, v, pctx, configPaths, leaf)
		if pointErr != nil {
			rest = append(rest, leaf, pointErr)

			continue
		}

		if !pointed {
			rest = append(rest, leaf)
		}
	}

	return errors.Join(rest...)
}

// pointAtOutputsError collects the diagnostic for leaf and reports whether leaf is dealt with.
func (p ComponentParser) pointAtOutputsError(
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	configPaths []string,
	leaf error,
) (bool, error) {
	if unknownErr, ok := errors.AsType[config.UnknownMockValueError](leaf); ok {
		return p.pointAtUnknownMockValue(l, v, pctx, unknownErr)
	}

	blockErr, ok := errors.AsType[config.DependencyBlockError](leaf)
	if !ok {
		return false, nil
	}

	readErr, ok := errors.AsType[config.DeclaredOutputsReadError](blockErr.Err)
	if !ok {
		return false, nil
	}

	if _, parsed := p.UnitConfigs[readErr.TargetConfig]; parsed {
		return true, nil
	}

	return p.pointAtDependencyRead(v.FS, configPaths, blockErr.Name, readErr)
}

// pointAtDependencyRead collects a diagnostic at the config_path of the dependency block named name in configPaths.
func (p ComponentParser) pointAtDependencyRead(
	fsys vfs.FS,
	configPaths []string,
	name string,
	readErr config.DeclaredOutputsReadError,
) (bool, error) {
	for _, configPath := range configPaths {
		source, err := readDependencySource(fsys, configPath)
		if err != nil {
			return false, err
		}

		expr, ok := source.configPaths[name]
		if !ok {
			continue
		}

		diag, err := diagnostic.NewDiagnostic(source.file, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryDependencyConfigUnreadable,
			Detail: fmt.Sprintf(
				"Reading the outputs block of %s failed: %s. Fix that file first.",
				readErr.TargetConfig,
				strings.TrimSuffix(readErr.Err.Error(), "."),
			),
			Subject: expr.Range().Ptr(),
		})
		if err != nil {
			return false, err
		}

		p.Collector.addDistinct(diag)

		return true, nil
	}

	return false, nil
}

// pointAtUnknownMockValue collects a diagnostic at the output block that declares the mock value unknownErr reports.
func (p ComponentParser) pointAtUnknownMockValue(
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	unknownErr config.UnknownMockValueError,
) (bool, error) {
	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, unknownErr.ConfigPath)
	if err != nil {
		return false, err
	}

	ranges := config.OutputDeclarationRanges(file)

	diag := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  summaryUnknownMockValue,
		Detail:   unknownErr.Error(),
		Subject:  declarationSubject(file, ranges, unknownErr.Output),
	}

	if err := p.Collector.keepDistinct(file.File, hcl.Diagnostics{diag}); err != nil {
		return false, err
	}

	return true, nil
}

// OutputsContractError reports that an outputs contract check of the unit at ConfigPath could not run.
type OutputsContractError struct {
	Err        error
	ConfigPath string
}

func (err OutputsContractError) Error() string {
	return fmt.Sprintf("checking the outputs contracts of %s: %v", err.ConfigPath, err.Err)
}

func (err OutputsContractError) Unwrap() error {
	return err.Err
}
