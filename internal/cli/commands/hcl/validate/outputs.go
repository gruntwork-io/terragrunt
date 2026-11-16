package validate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	summaryDeclaredOutputNotInModule  = "Declared output not in module"
	summaryUndeclaredDependencyOutput = "Undeclared dependency output"
	summaryUnknownMockValue           = "Unknown mock value"
)

type contractDiagnostics struct {
	file  *hclparse.File
	diags hcl.Diagnostics
}

type unitModule struct {
	dir   string
	files []string
}

type moduleLocation struct {
	copied util.CopyFilter
	dir    string
}

// moduleLoader returns the filesystem holding the module the unit of cfg runs, and the module in it.
type moduleLoader func(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (vfs.FS, unitModule, error)

// validateOutputsContracts collects a diagnostic per outputs contract the unit breaks, or [OutputsContractError].
func (p ComponentParser) validateOutputsContracts(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
) error {
	checks, err := outputsContractChecks(ctx, l, v, p.Store, opts, pctx, cfg, viewModule)
	if err != nil {
		return err
	}

	errs := make([]error, 0, len(checks))

	for _, check := range checks {
		p.Collector.dropEvaluationDiagnostics(check.diags)

		errs = append(errs, p.Collector.keepDistinct(check.file.File, check.diags))
	}

	return errors.Join(errs...)
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
	pctx := configbridge.NewParsingContext(opts)

	checks, err := outputsContractChecks(ctx, l, v, s, opts, pctx, cfg, preparedModule(moduleDir))
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

// outputsContractChecks returns the outputs contract diagnostics of the unit by file, none without --outputs.
func outputsContractChecks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	load moduleLoader,
) ([]contractDiagnostics, error) {
	if !opts.HCLValidateOutputs {
		return nil, nil
	}

	if !opts.Experiments.Evaluate(experiment.OutputsBlock) {
		return nil, ErrOutputsExperimentRequired
	}

	declared, err := declaredOutputsChecks(ctx, l, v, s, opts, pctx, cfg, load)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: opts.TerragruntConfigPath, Err: err}
	}

	references, err := referenceChecks(ctx, l, v, s, opts, pctx, cfg)
	if err != nil {
		return nil, err
	}

	return slices.Concat(declared, references), nil
}

// referenceChecks reports each reference of the unit and its includes to an output its dependency does not declare.
func referenceChecks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
) ([]contractDiagnostics, error) {
	contracts := readDependencyContracts(ctx, l, v, pctx, cfg)
	if !contracts.bind() {
		return nil, nil
	}

	configPath := opts.TerragruntConfigPath

	autoInclude, err := autoIncludeTree(ctx, l, v, s, pctx, configPath)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: configPath, Err: err}
	}

	replaced, err := autoIncludeReplacements(autoInclude)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: configPath, Err: err}
	}

	includedPaths, err := includedFiles(ctx, l, v, pctx, cfg, autoInclude)
	if err != nil {
		return nil, OutputsContractError{ConfigPath: configPath, Err: err}
	}

	checks := make([]contractDiagnostics, len(includedPaths)+1)
	errs := make([]error, len(includedPaths)+1)

	checks[0], errs[0] = fileReferenceCheck(ctx, l, v, s, pctx, cfg, contracts, configPath, replaced)

	for i, includedPath := range includedPaths {
		checks[i+1], errs[i+1] = fileReferenceCheck(ctx, l, v, s, pctx, cfg, contracts, includedPath, nil)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return checks, nil
}

// fileReferenceCheck reports each reference in the config at path to an output its dependency does not declare.
func fileReferenceCheck(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	contracts dependencyContracts,
	path string,
	replaced map[string]struct{},
) (contractDiagnostics, error) {
	file, err := syntaxTree(ctx, l, v, s, pctx, path)
	if err != nil {
		return contractDiagnostics{}, OutputsContractError{ConfigPath: path, Err: err}
	}

	diags, err := dependencyOutputReferenceDiagnostics(cfg, contracts, file, replaced)
	if err != nil {
		return contractDiagnostics{}, contractErrors(path, err)
	}

	return contractDiagnostics{file: file, diags: diags}, nil
}

// contractErrors wraps each error that err joins in an [OutputsContractError] of the config at path.
func contractErrors(path string, err error) error {
	leaves := joinedErrors(err)
	wrapped := make([]error, 0, len(leaves))

	for _, leaf := range leaves {
		wrapped = append(wrapped, OutputsContractError{ConfigPath: path, Err: leaf})
	}

	return errors.Join(wrapped...)
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

	paths = append(paths, autoInclude.ConfigPath)

	return append(paths, autoPaths...), nil
}

// mergedIncludePaths returns the paths of the includes that are merged or exposed, in label order.
func mergedIncludePaths(includes config.IncludeConfigsMap) ([]string, error) {
	paths := make([]string, 0, len(includes))
	errs := make([]error, 0, len(includes))

	for _, name := range slices.Sorted(maps.Keys(includes)) {
		include := includes[name]

		strategy, err := include.GetMergeStrategy()
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if strategy == config.NoMerge && !include.GetExpose() {
			continue
		}

		paths = append(paths, include.Path)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
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

// unitTFImpl returns the OpenTofu/Terraform implementation the unit runs, Unknown with an engine or a failed probe.
func unitTFImpl(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) tfimpl.Type {
	if cfg.Engine != nil {
		return tfimpl.Unknown
	}

	probeOpts := opts
	if !opts.TFPathExplicitlySet && cfg.TerraformBinary != "" {
		probeOpts = opts.Clone()
		probeOpts.TFPath = cfg.TerraformBinary
	}

	_, probe, err := configbridge.ProbeTF(ctx, l, v, probeOpts)
	if err != nil {
		l.Debugf(
			"Reading every module file of %s, the probe of %s failed: %v",
			opts.WorkingDir,
			tfimpl.BinaryOrDefault(v.Exec, probeOpts.TFPath),
			err,
		)

		return tfimpl.Unknown
	}

	return probe.Type
}

// declaredOutputsChecks reports each declared output the unit's module lacks, or its files that do not parse.
func declaredOutputsChecks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *rewrite.Store,
	opts *options.TerragruntOptions,
	pctx *config.ParsingContext,
	cfg *config.TerragruntConfig,
	load moduleLoader,
) ([]contractDiagnostics, error) {
	if cfg.Outputs == nil {
		return nil, nil
	}

	moduleFS, module, err := load(ctx, l, v, opts, cfg)
	if err != nil {
		return nil, err
	}

	moduleOutputs, err := tf.ModuleOutputs(moduleFS, module.files)
	if err != nil {
		var moduleDiags hcl.Diagnostics
		if !errors.As(err, &moduleDiags) {
			return nil, err
		}

		return moduleParseDiagnostics(moduleFS, module.files, moduleDiags)
	}

	missing := slices.DeleteFunc(cfg.Outputs.Names(), func(name string) bool {
		return slices.Contains(moduleOutputs, name)
	})
	if len(missing) == 0 {
		return nil, nil
	}

	file, err := syntaxTree(ctx, l, v, s, pctx, opts.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	return []contractDiagnostics{{file: file, diags: missingOutputDiagnostics(file, module.dir, missing)}}, nil
}

// missingOutputDiagnostics returns a diagnostic at the declaration in file of each output the module at dir lacks.
func missingOutputDiagnostics(file *hclparse.File, dir string, missing []string) hcl.Diagnostics {
	ranges := config.OutputDeclarationRanges(file)
	diags := make(hcl.Diagnostics, 0, len(missing))

	for _, name := range missing {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  summaryDeclaredOutputNotInModule,
			Detail: fmt.Sprintf(
				"The outputs block declares output %q, but the OpenTofu/Terraform module at %s has no output %q. "+
					"Remove the declaration or add the output to the module.",
				name,
				dir,
				name,
			),
			Subject: declarationSubject(file, ranges, name),
		})
	}

	return diags
}

// moduleParseDiagnostics reports each of files in moduleFS that fails to parse, or moduleDiags when none does.
func moduleParseDiagnostics(
	moduleFS vfs.FS,
	files []string,
	moduleDiags hcl.Diagnostics,
) ([]contractDiagnostics, error) {
	checks := make([]contractDiagnostics, 0, len(files))
	errs := make([]error, 0, len(files))

	for _, path := range files {
		src, err := vfs.ReadFile(moduleFS, path)
		if err != nil {
			errs = append(errs, err)

			continue
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

	if err := errors.Join(errs...); err != nil {
		return nil, err
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

// viewModule returns an in-memory overlay of the module the unit runs, generate blocks written on top.
func viewModule(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (vfs.FS, unitModule, error) {
	impl := unitTFImpl(ctx, l, v, opts, cfg)
	runCfg := cfg.ToRunConfig(l, v.FS)

	source, err := moduleSource(ctx, l, v, opts, runCfg, impl)
	if err != nil {
		return nil, unitModule{}, err
	}

	view := vfs.NewOverlayFS(v.FS, vfs.NewMemMapFS())

	if err := generateInView(ctx, l, v.WithFS(view), source.dir, runCfg); err != nil {
		return nil, unitModule{}, err
	}

	files, err := moduleFiles(view, source, impl)
	if err != nil {
		return nil, unitModule{}, err
	}

	return view, unitModule{dir: source.dir, files: files}, nil
}

// preparedModule returns the loader of a module that a run has already prepared at dir.
func preparedModule(dir string) moduleLoader {
	return func(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		opts *options.TerragruntOptions,
		cfg *config.TerragruntConfig,
	) (vfs.FS, unitModule, error) {
		source := moduleLocation{dir: dir, copied: keepEveryFile}

		files, err := moduleFiles(v.FS, source, unitTFImpl(ctx, l, v, opts, cfg))
		if err != nil {
			return nil, unitModule{}, err
		}

		return v.FS, unitModule{dir: dir, files: files}, nil
	}
}

// moduleFiles returns the files of the module at source in fsys that the run copies and impl reads.
func moduleFiles(fsys vfs.FS, source moduleLocation, impl tfimpl.Type) ([]string, error) {
	paths, err := util.ListTfFiles(fsys, source.dir)
	if err != nil {
		return nil, err
	}

	copied := slices.DeleteFunc(paths, func(path string) bool {
		return !source.copied(path, false)
	})

	return tf.ImplFiles(copied, impl), nil
}

// moduleSource returns where the module the unit runs is, and the filter of the files the run copies from there.
func moduleSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	impl tfimpl.Type,
) (moduleLocation, error) {
	inPlace, err := runcfg.ShouldSkipCache(
		opts.Experiments.Evaluate(experiment.NoDotTerragruntCache),
		opts.Source,
		opts.TerragruntConfigPath,
		runCfg,
	)
	if err != nil {
		return moduleLocation{}, err
	}

	if inPlace {
		return moduleLocation{dir: opts.WorkingDir, copied: keepEveryFile}, nil
	}

	if opts.Source == "" && runCfg.Terraform.Source == "" {
		return unitDirectoryModule(l, v, opts, runCfg, impl)
	}

	sourceURL, err := runcfg.GetTerraformSourceURL(
		opts.Source,
		opts.SourceMap,
		opts.OriginalTerragruntConfigPath,
		runCfg,
	)
	if err != nil {
		return moduleLocation{}, err
	}

	moduleDir, err := downloadModule(ctx, l, v, opts, runCfg, sourceURL, impl)
	if err != nil {
		return moduleLocation{}, err
	}

	return moduleLocation{dir: moduleDir, copied: keepEveryFile}, nil
}

// unitDirectoryModule returns the module of a unit without a source: its own directory, less what the run leaves out.
func unitDirectoryModule(
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	impl tfimpl.Type,
) (moduleLocation, error) {
	copyOpts := run.ModuleCopyOptions(configbridge.NewRunOptions(opts, impl), runCfg)

	copied, err := util.NewCopyFilter(l, v.FS, opts.WorkingDir, copyOpts...)
	if err != nil {
		return moduleLocation{}, err
	}

	return moduleLocation{dir: opts.WorkingDir, copied: copied}, nil
}

// downloadModule fetches sourceURL as impl and the unit's role do, hooks left out, and returns the module dir.
func downloadModule(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	runCfg *runcfg.RunConfig,
	sourceURL string,
	impl tfimpl.Type,
) (string, error) {
	v = v.WithEnvCloned()
	getter := creds.NewGetter()

	provider := externalcmd.NewProvider(l, opts.AuthProviderCmd, configbridge.ShellRunOptsFromOpts(v.Exec, v.Env, opts))
	if err := getter.ObtainAndUpdateEnvIfNecessary(ctx, l, v, provider); err != nil {
		return "", err
	}

	downloadOpts := opts.Clone()
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
		configbridge.NewRunOptions(downloadOpts, impl),
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

// generateInView writes the generate blocks and the remote_state backend file of runCfg into the module at moduleDir.
func generateInView(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	moduleDir string,
	runCfg *runcfg.RunConfig,
) error {
	errs := make([]error, 0, len(runCfg.GenerateConfigs)+1)

	for _, name := range slices.Sorted(maps.Keys(runCfg.GenerateConfigs)) {
		genCfg := runCfg.GenerateConfigs[name]

		errs = append(errs, codegen.WriteToFile(ctx, l, v, moduleDir, &genCfg))
	}

	if runCfg.RemoteState.Config != nil && runCfg.RemoteState.Generate != nil {
		errs = append(errs, runCfg.RemoteState.GenerateOpenTofuCode(ctx, l, v, moduleDir))
	}

	return errors.Join(errs...)
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

	unread := map[string]error{}

	for _, reference := range references {
		address := reference.Address()

		contract := contracts[address]
		if contract.err != nil {
			unread[address] = contract.err

			continue
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

	errs := make([]error, 0, len(unread))

	for _, address := range slices.Sorted(maps.Keys(unread)) {
		errs = append(errs, unread[address])
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
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
	err error,
) error {
	if err == nil {
		return nil
	}

	var rest []error

	for _, leaf := range joinedErrors(err) {
		unknownErr, ok := errors.AsType[config.UnknownMockValueError](leaf)
		if !ok {
			rest = append(rest, leaf)

			continue
		}

		if pointErr := p.pointAtUnknownMockValue(l, v, pctx, unknownErr); pointErr != nil {
			rest = append(rest, leaf, pointErr)
		}
	}

	return errors.Join(rest...)
}

// pointAtUnknownMockValue collects a diagnostic at the output block that declares the mock value unknownErr reports.
func (p ComponentParser) pointAtUnknownMockValue(
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	unknownErr config.UnknownMockValueError,
) error {
	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, unknownErr.ConfigPath)
	if err != nil {
		return err
	}

	ranges := config.OutputDeclarationRanges(file)

	diag := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  summaryUnknownMockValue,
		Detail:   unknownErr.Error(),
		Subject:  declarationSubject(file, ranges, unknownErr.Output),
	}

	return p.Collector.keepDistinct(file.File, hcl.Diagnostics{diag})
}

// ErrOutputsExperimentRequired is returned when --outputs is set without the outputs-block experiment enabled.
var ErrOutputsExperimentRequired = errors.New(
	"the '--" + OutputsFlagName + "' flag requires the '" + experiment.OutputsBlock + "' experiment to be enabled" +
		" (set --experiment " + experiment.OutputsBlock + " or TG_EXPERIMENT=" + experiment.OutputsBlock + ")",
)

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
