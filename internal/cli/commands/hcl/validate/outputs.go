package validate

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/gruntwork-io/terragrunt/internal/prepare"
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
	// summaryDependencyConfigUnreadable heads a diagnostic for a dependency whose unit config cannot be read for
	// its outputs block.
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

// validateOutputsContracts checks the outputs contracts of the unit at opts.TerragruntConfigPath under the
// outputs-block experiment: every output its outputs block declares exists in its OpenTofu/Terraform module, and
// every dependency output it reads, in its own file or in a file it merges or exposes, is declared by the
// dependency, when that dependency declares outputs. It collects a diagnostic per failed check, against the file
// that holds the declaration or the reference, or against a module file that does not parse, and does nothing
// when the experiment is off. The module is generated in memory, so no generated file lands in the module
// directory.
//
// Returns [OutputsContractError] when a check cannot run.
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
		if err := p.Collector.keepDistinct(check.file.File, check.diags); err != nil {
			return err
		}
	}

	return nil
}

// validateOutputsContractsPrepared runs the outputs contract checks of the unit at opts.TerragruntConfigPath
// against its module prepared on disk at moduleDir, as hcl validate --inputs prepares it, and returns the
// diagnostics of every file as one error. It does nothing when the experiment is off.
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

// outputsContractDiagnostics runs the outputs contract checks of the unit at opts.TerragruntConfigPath and returns
// the diagnostics by file: the module files that do not parse, then the unit file, then each file it merges or
// exposes and its autoinclude file. The declared outputs are checked against the module in moduleFS at moduleDir,
// which is read only when the unit declares outputs. A dependency block of the unit file that its autoinclude file
// replaces is not read.
//
// Returns [OutputsContractError] when a check cannot run, wrapping a [config.DependencyBlockError] when the
// outputs block of a dependency cannot be read.
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
		moduleChecks, err := declaredOutputsDiagnostics(moduleFS, moduleDir, moduleEngine(opts), cfg, file)
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

// includedFiles returns the files the unit at configPath evaluates besides its own: the files of its include
// blocks that it merges or exposes, in label order, and the sibling autoinclude file when there is one. An
// include with the no_merge strategy that is not exposed is never evaluated, so its references are not checked.
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

// autoIncludeReplacements returns the labels of the dependency blocks the autoinclude file beside configPath
// declares without an expansion. Each replaces the block of the unit file with the same label whole, so the parse
// never reads that one. It returns none when there is no autoinclude file.
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

// moduleEngine returns the OpenTofu/Terraform implementation the unit runs: the one opts resolved, or else the one
// the binary name of opts.TFPath says.
func moduleEngine(opts *options.TerragruntOptions) tfimpl.Type {
	if opts.TofuImplementation != "" && opts.TofuImplementation != tfimpl.Unknown {
		return opts.TofuImplementation
	}

	return tfimpl.FromPath(opts.TFPath)
}

// declaredOutputsDiagnostics reports each output the unit declares that the OpenTofu/Terraform module in moduleFS
// at moduleDir, read as engine reads it, does not, against the unit file. A module file that does not parse is
// reported instead, against that file.
func declaredOutputsDiagnostics(
	moduleFS vfs.FS,
	moduleDir string,
	engine tfimpl.Type,
	cfg *config.TerragruntConfig,
	file *hclparse.File,
) ([]contractDiagnostics, error) {
	moduleOutputs, err := tf.ModuleOutputs(moduleFS, moduleDir, engine)
	if err != nil {
		if !errors.As(err, new(hcl.Diagnostics)) {
			return nil, err
		}

		return moduleParseDiagnostics(moduleFS, moduleDir, engine, err)
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

// moduleParseDiagnostics parses each OpenTofu/Terraform file of the module at moduleDir that engine reads on its
// own and returns the diagnostics of each file that fails, against that file: the diagnostics its own parse
// raises, or those of parseErr, the error of reading the module whole, that point at it. It returns parseErr when
// no file is to blame.
func moduleParseDiagnostics(
	moduleFS vfs.FS,
	moduleDir string,
	engine tfimpl.Type,
	parseErr error,
) ([]contractDiagnostics, error) {
	paths, err := util.ListTfFiles(moduleFS, moduleDir)
	if err != nil {
		return nil, err
	}

	paths = tf.EngineFiles(paths, engine)

	var moduleDiags hcl.Diagnostics

	if !errors.As(parseErr, &moduleDiags) {
		return nil, parseErr
	}

	var checks []contractDiagnostics

	for _, path := range paths {
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
		return nil, parseErr
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

// moduleView returns an in-memory view of the OpenTofu/Terraform module the unit runs, and the module directory
// in it. A unit with a source, or a run with --source, is fetched into the download directory first; a unit
// without one is viewed through the filter of its copy into the cache directory, unless it runs in place. The
// view has the OpenTofu/Terraform files of the module directory and the files its generate blocks target, with the
// generate blocks and the remote_state backend file written on top, so nothing is written into the module
// directory.
func moduleView(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) (vfs.FS, string, error) {
	runCfg := cfg.ToRunConfig(l, v.FS)

	moduleOpts, copyFilter, err := moduleSource(ctx, l, v, opts, cfg, runCfg)
	if err != nil {
		return nil, "", err
	}

	view := vfs.NewMemMapFS()

	if err := copyModuleFiles(v.FS, view, moduleOpts.WorkingDir, runCfg, copyFilter); err != nil {
		return nil, "", err
	}

	if err := generateInView(ctx, l, v.WithFS(view), moduleOpts.WorkingDir, runCfg); err != nil {
		return nil, "", err
	}

	return view, moduleOpts.WorkingDir, nil
}

// moduleSource returns the options whose working directory holds the module the unit runs, and the filter of the
// files the run copies from that directory into the cache directory. A unit with a source, or a run with --source,
// is fetched into the download directory, which holds the copy already, and a unit that runs in place is not
// copied, so both keep every file.
func moduleSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	runCfg *runcfg.RunConfig,
) (*options.TerragruntOptions, util.CopyFilter, error) {
	hasSource := cfg.Terraform != nil && cfg.Terraform.Source != nil && *cfg.Terraform.Source != ""
	if hasSource || opts.Source != "" {
		prepared, err := prepare.PrepareSource(ctx, l, v, opts, cfg, report.NewReport())
		if err != nil {
			return nil, nil, err
		}

		return prepared, keepEveryFile, nil
	}

	inPlace, err := runcfg.ShouldSkipCache(
		opts.Experiments.Evaluate(experiment.NoDotTerragruntCache),
		opts.Source,
		opts.TerragruntConfigPath,
		runCfg,
	)
	if err != nil {
		return nil, nil, err
	}

	if inPlace {
		return opts, keepEveryFile, nil
	}

	copyOpts := run.ModuleCopyOptions(configbridge.NewRunOptions(opts), runCfg)

	filter, err := util.NewCopyFilter(l, v.FS, opts.WorkingDir, copyOpts...)
	if err != nil {
		return nil, nil, err
	}

	return opts, filter, nil
}

// keepEveryFile is the copy filter of a module the run reads as it is.
func keepEveryFile(string, bool) bool {
	return true
}

// copyModuleFiles copies the OpenTofu/Terraform files of the module at dir, and the files the generate blocks of
// runCfg target when they exist, from source into view at the same paths, leaving out the files filter rejects.
// A symbolic link is copied as the file it points at.
func copyModuleFiles(source, view vfs.FS, dir string, runCfg *runcfg.RunConfig, filter util.CopyFilter) error {
	paths, err := util.ListTfFiles(source, dir)
	if err != nil {
		return err
	}

	for _, target := range generateTargets(dir, runCfg) {
		if info, err := source.Stat(target); err == nil && info.Mode().IsRegular() {
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

// generateTargets returns the paths that the generate blocks and the remote_state block of runCfg write, as
// generation resolves them: an absolute path as it is, any other under dir.
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

// generateInView writes the generate blocks of runCfg, and its remote_state backend file when the block asks for
// one, into the module at moduleDir on the filesystem of v.
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
// does not declare. A dependency that declares no outputs block is not checked. References resolve against the
// unit's merged dependency blocks, so a dependency an autoinclude overrides is checked as the autoinclude declares
// it, and the dependency blocks of file labeled in replaced, which the autoinclude replaces whole, are not read.
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

// declaredOutputsOf returns the outputs block of the unit the dependency at address reads, or nil when there is
// no such dependency or it reads no outputs block.
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

// pointAtOutputsErrors moves the outputs contract errors in err's tree into diagnostics: a dependency whose unit
// config cannot be read for its outputs block is reported at the config_path expression that declares it, in the
// first of configPaths that does, unless the run parses that unit config itself, and a declared mock value that
// its readers cannot know is reported at its output block, in the unit config that declares it. It returns the
// errors it found no place for, with any failure to place one.
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

// pointAtOutputsError collects the diagnostic for leaf when it is an outputs contract error with a place to point
// at, and reports whether leaf is dealt with: pointed at, or left to the parse of the unit config it is about.
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

// pointAtDependencyRead collects a diagnostic at the config_path expression of the dependency block named name,
// in the first of configPaths that declares it, saying why the unit config it names could not be read.
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
				"Reading the outputs block of %s failed: %v. Fix that file first.",
				readErr.TargetConfig,
				readErr.Err,
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

// pointAtUnknownMockValue collects a diagnostic at the output block of the unit config that declares the mock
// value unknownErr reports.
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
