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
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
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
	summaryDeclaredOutputNotInModule  = "Declared output not in module"
	summaryUndeclaredDependencyOutput = "Undeclared dependency output"
	summaryDependencyConfigUnreadable = "Dependency config not readable"
	summaryUnknownMockValue           = "Unknown mock value"
)

const viewFilePerms = 0o644

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
	if !opts.HCLValidateOutputs || !opts.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil
	}

	var module unitModule

	if cfg.Outputs != nil {
		var err error

		module, err = moduleView(ctx, l, v, opts, cfg)
		if err != nil {
			return OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
		}
	}

	checks, err := outputsContractDiagnostics(ctx, l, v, p.Store, opts, pctx, cfg, module)
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
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	moduleDir string,
) error {
	if !opts.HCLValidateOutputs || !opts.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil
	}

	pctx := configbridge.NewParsingContext(opts)
	module := unitModule{fs: v.FS, dir: moduleDir}

	if cfg.Outputs != nil {
		module.engine = moduleEngine(ctx, l, v, opts, cfg)
	}

	checks, err := outputsContractDiagnostics(ctx, l, v, s, opts, pctx, cfg, module)
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
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	module unitModule,
) ([]contractDiagnostics, error) {
	contracts := readDependencyContracts(ctx, l, v, pctx, cfg)
	if cfg.Outputs == nil && !contracts.bind() {
		return nil, nil
	}

	unitFile := func() (*hclparse.File, error) {
		return syntaxTree(ctx, l, v, s, pctx, opts.TerragruntConfigPath)
	}

	var checks []contractDiagnostics

	if cfg.Outputs != nil {
		moduleChecks, err := declaredOutputsDiagnostics(module, cfg, unitFile)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
		}

		checks = append(checks, moduleChecks...)
	}

	if !contracts.bind() {
		return checks, nil
	}

	file, err := unitFile()
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	autoInclude, err := autoIncludeTree(ctx, l, v, s, pctx, opts.TerragruntConfigPath)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	replaced, err := autoIncludeReplacements(autoInclude)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	referenceDiags, err := dependencyOutputReferenceDiagnostics(cfg, contracts, file, replaced)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	checks = append(checks, contractDiagnostics{file: file, diags: referenceDiags})

	includedPaths, err := includedFiles(ctx, l, v, pctx, cfg, autoInclude)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	for _, includedPath := range includedPaths {
		included, err := syntaxTree(ctx, l, v, s, pctx, includedPath)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: includedPath, Err: err}
		}

		includedDiags, err := dependencyOutputReferenceDiagnostics(cfg, contracts, included, nil)
		if err != nil {
			return nil, OutputsContractError{ConfigPath: includedPath, Err: err}
		}

		checks = append(checks, contractDiagnostics{file: included, diags: includedDiags})
	}

	return checks, nil
}

// syntaxTree returns the syntax tree of the config at path, parsed once for the run by the parse store.
func syntaxTree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	pctx *config.ParsingContext,
	path string,
) (*hclparse.File, error) {
	info, err := v.FS.Stat(path)
	if err != nil {
		return nil, err
	}

	return s.ParseUnitAST(ctx, v.FS, path, info, pctx.NewParser(l, v))
}

// autoIncludeTree returns the syntax tree of the autoinclude file beside configPath, or nil when there is none.
func autoIncludeTree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	pctx *config.ParsingContext,
	configPath string,
) (*hclparse.File, error) {
	autoInclude := filepath.Join(filepath.Dir(configPath), config.DefaultAutoIncludeFile)

	info, err := v.FS.Stat(autoInclude)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return s.ParseUnitAST(ctx, v.FS, autoInclude, info, pctx.NewParser(l, v))
}

// includedFiles returns the merged or exposed include files of the unit, its autoinclude file and theirs.
func includedFiles(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	autoInclude *hclparse.File,
) ([]string, error) {
	paths, err := mergedIncludePaths(cfg.ProcessedIncludes)
	if err != nil {
		return nil, err
	}

	if autoInclude == nil {
		return paths, nil
	}

	l, autoPctx, err := pctx.WithConfigPath(l, autoInclude.ConfigPath)
	if err != nil {
		return nil, err
	}

	autoPctx.SkipAutoIncludeMerge = true

	autoCfg, err := config.PartialParseConfigFile(
		ctx,
		l,
		v,
		autoPctx.WithDecodeList(config.TerragruntFlags),
		autoInclude.ConfigPath,
		nil,
	)
	if err != nil {
		return nil, err
	}

	autoPaths, err := mergedIncludePaths(autoCfg.ProcessedIncludes)
	if err != nil {
		return nil, err
	}

	return slices.Concat(paths, []string{autoInclude.ConfigPath}, autoPaths), nil
}

// mergedIncludePaths returns the paths of the includes that are merged or exposed, in label order.
func mergedIncludePaths(includes config.IncludeConfigsMap) ([]string, error) {
	paths := make([]string, 0, len(includes))

	for _, name := range slices.Sorted(maps.Keys(includes)) {
		include := includes[name]

		strategy, err := include.GetMergeStrategy()
		if err != nil {
			return nil, err
		}

		if strategy == config.NoMerge && !include.GetExpose() {
			continue
		}

		paths = append(paths, include.Path)
	}

	return paths, nil
}

// autoIncludeReplacements returns the unexpanded dependency labels of the autoinclude file, none when it is nil.
func autoIncludeReplacements(autoInclude *hclparse.File) (map[string]struct{}, error) {
	if autoInclude == nil {
		return nil, nil
	}

	return autoInclude.UnexpandedLabels(config.MetadataDependency, &config.Dependency{})
}

// moduleEngine returns the implementation of the binary the unit runs, probed as a run probes it, or Unknown.
func moduleEngine(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) tfimpl.Type {
	engineOpts := opts
	if !opts.TFPathExplicitlySet && cfg.TerraformBinary != "" {
		engineOpts = opts.Clone()
		engineOpts.TFPath = cfg.TerraformBinary
	}

	resolved := opts.TofuImplementation != "" && opts.TofuImplementation != tfimpl.Unknown
	if engineOpts == opts && resolved {
		return opts.TofuImplementation
	}

	_, _, impl, err := run.PopulateTFVersion(ctx, l, v, run.PopulateTFVersionInput{
		TFOpts:       configbridge.TFRunOptsFromOpts(v.Env, engineOpts),
		WorkingDir:   opts.WorkingDir,
		VersionFiles: opts.VersionManagerFileName,
	})
	if err != nil {
		l.Debugf("Reading every module file of %s, the probe of %s failed: %v", opts.WorkingDir, engineOpts.TFPath, err)

		return tfimpl.Unknown
	}

	return impl
}

// declaredOutputsDiagnostics reports each declared output the module lacks, or the module files that do not parse.
func declaredOutputsDiagnostics(
	module unitModule,
	cfg *config.TerragruntConfig,
	unitFile func() (*hclparse.File, error),
) ([]contractDiagnostics, error) {
	moduleOutputs, err := tf.ModuleOutputs(module.fs, module.dir, module.engine)
	if err != nil {
		var moduleDiags hcl.Diagnostics
		if !errors.As(err, &moduleDiags) {
			return nil, err
		}

		return moduleParseDiagnostics(module, moduleDiags)
	}

	missing := slices.DeleteFunc(cfg.Outputs.Names(), func(name string) bool {
		return slices.Contains(moduleOutputs, name)
	})
	if len(missing) == 0 {
		return nil, nil
	}

	file, err := unitFile()
	if err != nil {
		return nil, err
	}

	ranges := config.OutputDeclarationRanges(file)

	var diags hcl.Diagnostics

	for _, name := range missing {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryDeclaredOutputNotInModule,
			Detail: fmt.Sprintf(
				"The outputs block declares output %q, but the OpenTofu/Terraform module at %s has no output %q. "+
					"Remove the declaration or add the output to the module.",
				name,
				module.dir,
				name,
			),
			Subject: declarationSubject(file, ranges, name),
		})
	}

	return []contractDiagnostics{{file: file, diags: diags}}, nil
}

// moduleParseDiagnostics reports each module file engine reads that fails to parse, or moduleDiags when none does.
func moduleParseDiagnostics(module unitModule, moduleDiags hcl.Diagnostics) ([]contractDiagnostics, error) {
	paths, err := util.ListTfFiles(module.fs, module.dir)
	if err != nil {
		return nil, err
	}

	var checks []contractDiagnostics

	for _, path := range tf.EngineFiles(paths, module.engine) {
		src, err := vfs.ReadFile(module.fs, path)
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

type unitModule struct {
	fs     vfs.FS
	dir    string
	engine tfimpl.Type
}

// moduleView returns the module the unit runs as an in-memory view, generate blocks written on top, with its engine.
func moduleView(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (unitModule, error) {
	engine := moduleEngine(ctx, l, v, opts, cfg)
	runCfg := cfg.ToRunConfig(l, v.FS)

	moduleDir, copyFilter, err := moduleSource(ctx, l, v, opts, runCfg, engine)
	if err != nil {
		return unitModule{}, err
	}

	view := vfs.NewMemMapFS()

	if err := copyModuleFiles(v.FS, view, moduleDir, runCfg, copyFilter); err != nil {
		return unitModule{}, err
	}

	if err := generateInView(ctx, l, v.WithFS(view), moduleDir, runCfg); err != nil {
		return unitModule{}, err
	}

	return unitModule{fs: view, dir: moduleDir, engine: engine}, nil
}

// moduleSource returns the directory holding the module the unit runs and the filter of the files the run copies.
func moduleSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	engine tfimpl.Type,
) (string, util.CopyFilter, error) {
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
		moduleDir, err := downloadModule(ctx, l, v, opts, runCfg, sourceURL, engine)

		return moduleDir, keepEveryFile, err
	}

	copyOpts := run.ModuleCopyOptions(configbridge.NewRunOptions(opts), runCfg)

	filter, err := util.NewCopyFilter(l, v.FS, opts.WorkingDir, copyOpts...)
	if err != nil {
		return "", nil, err
	}

	return opts.WorkingDir, filter, nil
}

const unitDirSource = "."

// downloadModule fetches sourceURL as the unit's engine and role do, hooks left out, and returns the module dir.
func downloadModule(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	sourceURL string,
	engine tfimpl.Type,
) (string, error) {
	v = v.WithEnvCloned()
	getter := creds.NewGetter()

	provider := externalcmd.NewProvider(l, opts.AuthProviderCmd, configbridge.ShellRunOptsFromOpts(v.Env, opts))
	if err := getter.ObtainAndUpdateEnvIfNecessary(ctx, l, v, provider); err != nil {
		return "", err
	}

	downloadOpts := opts.Clone()
	downloadOpts.TofuImplementation = engine
	downloadOpts.IAMRoleOptions = iam.MergeRoleOptions(runCfg.GetIAMRoleOptions(), opts.OriginalIAMRoleOptions)

	role := amazonsts.NewProvider(l, downloadOpts.IAMRoleOptions, v.Env)
	if err := getter.ObtainAndUpdateEnvIfNecessary(ctx, l, v, role); err != nil {
		return "", err
	}

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
	cfg *config.TerragruntConfig,
	contracts dependencyContracts,
	file *hclparse.File,
	replaced map[string]struct{},
) (hcl.Diagnostics, error) {
	references, err := config.DependencyOutputReferences(file, cfg.TerragruntDependencies, replaced)
	if err != nil {
		return nil, err
	}

	var diags hcl.Diagnostics

	for _, reference := range references {
		address := reference.Address()

		contract := contracts[address]
		if contract.err != nil {
			return nil, contract.err
		}

		if contract.declared == nil || contract.declared.Declares(reference.Output) {
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
				strings.Join(contract.declared.Names(), ", "),
				reference.Output,
			),
			Subject: &reference.Range,
		})
	}

	return diags, nil
}

type dependencyContract struct {
	declared *config.OutputsConfig
	err      error
}

type dependencyContracts map[string]dependencyContract

// bind reports whether a reference can break a contract: one declares outputs or could not be read.
func (contracts dependencyContracts) bind() bool {
	for _, contract := range contracts {
		if contract.err != nil || contract.declared != nil {
			return true
		}
	}

	return false
}

// readDependencyContracts returns what the unit each dependency of cfg reads declares, by dependency address.
func readDependencyContracts(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
) dependencyContracts {
	contracts := make(dependencyContracts, len(cfg.TerragruntDependencies))

	for i := range cfg.TerragruntDependencies {
		dep := &cfg.TerragruntDependencies[i]

		address := dep.Address()
		if _, read := contracts[address]; read {
			continue
		}

		declared, err := dep.DeclaredOutputs(ctx, l, v, pctx)
		contracts[address] = dependencyContract{declared: declared, err: err}
	}

	return contracts
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

// OutputsExperimentRequiredError reports that --outputs was set without the outputs-block experiment.
type OutputsExperimentRequiredError struct{}

func (err OutputsExperimentRequiredError) Error() string {
	return "the --" + OutputsFlagName + " flag needs the " + experiment.OutputsBlock + " experiment. " +
		"Enable it with --experiment " + experiment.OutputsBlock + " or TG_EXPERIMENT=" + experiment.OutputsBlock + "."
}

// OutputsContractError reports that checking the outputs contracts of ConfigPath failed with Err.
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
