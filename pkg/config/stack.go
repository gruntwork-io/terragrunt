package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/worker"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/hashicorp/hcl/v2/hclwrite"

	"github.com/zclconf/go-cty/cty"

	"errors"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const (
	// StackDir aliases inthclparse.StackDir so external callers (internal/stacks/output, etc.) keep their existing import path without a second source of truth.
	StackDir      = inthclparse.StackDir
	valuesFile    = "terragrunt.values.hcl"
	manifestName  = ".terragrunt-stack-manifest"
	unitDirPerm   = 0755
	valueFilePerm = 0644
)

// StackConfigFile represents the structure of terragrunt.stack.hcl stack file.
type StackConfigFile struct {
	Locals   *terragruntLocal    `hcl:"locals,block"`
	Includes []*StackIncludeFile `hcl:"include,block"`
	Stacks   []*Stack            `hcl:"stack,block"`
	Units    []*Unit             `hcl:"unit,block"`
}

// StackIncludeFile represents an include block in a stack file.
type StackIncludeFile struct {
	Name string `hcl:",label"`
	Path string `hcl:"path,attr"`
}

// StackConfig represents the structure of terragrunt.stack.hcl stack file.
type StackConfig struct {
	Locals map[string]any
	Stacks []*Stack
	Units  []*Unit
}

// Unit represents unit from a stack file.
type Unit struct {
	Remain              hcl.Body   `hcl:",remain"`
	UpdateSourceWithCAS *bool      `hcl:"update_source_with_cas,attr"`
	Mutable             *bool      `hcl:"mutable,attr"`
	NoStack             *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation        *bool      `hcl:"no_validation,attr"`
	Values              *cty.Value `hcl:"values,attr"`
	Name                string     `hcl:",label"`
	Source              string     `hcl:"source,attr"`
	Path                string     `hcl:"path,attr"`
}

// GeneratedPath returns the on-disk path this unit generates to under stackDir.
func (u *Unit) GeneratedPath(stackDir string) string {
	return inthclparse.GeneratedComponentPath(stackDir, u.Path, u.NoStack != nil && *u.NoStack)
}

// Stack represents the stack block in the configuration.
type Stack struct {
	Remain              hcl.Body   `hcl:",remain"`
	UpdateSourceWithCAS *bool      `hcl:"update_source_with_cas,attr"`
	Mutable             *bool      `hcl:"mutable,attr"`
	NoStack             *bool      `hcl:"no_dot_terragrunt_stack,attr"`
	NoValidation        *bool      `hcl:"no_validation,attr"`
	Values              *cty.Value `hcl:"values,attr"`
	Name                string     `hcl:",label"`
	Source              string     `hcl:"source,attr"`
	Path                string     `hcl:"path,attr"`
}

// GeneratedPath returns the on-disk path this stack generates to under stackDir.
func (s *Stack) GeneratedPath(stackDir string) string {
	return inthclparse.GeneratedComponentPath(stackDir, s.Path, s.NoStack != nil && *s.NoStack)
}

// GenerateStackFile generates the Terragrunt stack configuration from the given stackFilePath,
// reads necessary values, and generates units and stacks in the target directory.
// It handles the creation of required directories and returns any errors encountered.
func GenerateStackFile(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	pool *worker.Pool,
	stackFilePath string,
) error {
	stackSourceDir := filepath.Dir(stackFilePath)

	values, err := ReadValues(ctx, pctx, l, stackSourceDir)
	if err != nil {
		return fmt.Errorf("failed to read values from directory %s: %w", stackSourceDir, err)
	}

	stackFile, err := ReadStackConfigFile(ctx, l, pctx, stackFilePath, values)
	if err != nil {
		return fmt.Errorf(
			"failed to read stack file %s in %s %w",
			stackFilePath,
			stackSourceDir,
			err,
		)
	}

	stackTargetDir := filepath.Join(stackSourceDir, StackDir)

	// Perform a two-pass parse to resolve autoinclude blocks and generate
	// terragrunt.autoinclude.hcl files.
	autoIncludes, stackSrcBytes, err := resolveStackAutoIncludes(
		ctx,
		l,
		pctx,
		stackFilePath,
		stackFile,
		values,
	)
	if err != nil {
		return err
	}

	casEnabled := !pctx.NoCAS

	if err := validateUpdateSourceWithCAS(stackFile, stackFilePath, casEnabled); err != nil {
		return err
	}

	cs, err := setupCAS(l, casEnabled, pctx.CASCloneDepth)
	if err != nil {
		return err
	}

	genOpts := generateOpts{
		rootWorkingDir:  pctx.RootWorkingDir,
		logShowAbsPaths: pctx.LogShowAbsPaths,
		sourceMap:       pctx.SourceMap,
		noStackValidate: pctx.NoStackValidate,
		stackConfigPath: pctx.TerragruntStackConfigPath,
		sourceFile:      stackFilePath,
		sourceDir:       stackSourceDir,
		targetDir:       stackTargetDir,
		autoIncludes:    autoIncludes,
		stackSrcBytes:   stackSrcBytes,
		casEnabled:      cs.Enabled,
		casInstance:     cs.Instance,
		casVenv:         cs.Venv,
		strictControls:  pctx.StrictControls,
	}

	fs := pctx.Venv.FS

	if err := generateUnits(ctx, l, fs, &genOpts, pool, stackFile.Units); err != nil {
		return err
	}

	if err := generateStacks(ctx, l, fs, &genOpts, pool, stackFile.Stacks); err != nil {
		return err
	}

	return nil
}

// ValidateStackAutoIncludes runs the strict autoinclude parse over the stack file at
// stackFilePath without generating anything, so tooling such as `hcl validate` reports
// the same autoinclude errors [GenerateStackFile] would raise. stackFile is the config
// already parsed from stackFilePath via [ParseStackConfig]. It is a no-op when no unit
// or stack declares autoinclude.
func ValidateStackAutoIncludes(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	stackFilePath string,
	stackFile *StackConfig,
	values *cty.Value,
) error {
	_, _, err := resolveStackAutoIncludes(ctx, l, pctx, stackFilePath, stackFile, values)

	return err
}

// resolveStackAutoIncludes runs the strict phased autoinclude parse over the stack file at
// stackFilePath and returns the resolved autoincludes keyed by [inthclparse.AutoIncludeKey],
// plus the raw stack file bytes the generator slices expression ranges from. It returns nil
// results when stackFile declares no autoinclude blocks.
func resolveStackAutoIncludes(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	stackFilePath string,
	stackFile *StackConfig,
	values *cty.Value,
) (map[string]*inthclparse.AutoIncludeResolved, []byte, error) {
	if !stackConfigHasAutoInclude(stackFile) {
		return nil, nil, nil
	}

	stackSourceDir := filepath.Dir(stackFilePath)

	// The autoinclude phase uses the internal HCL parser, which currently expects
	// hclsyntax input (not JSON bodies). Preserve explicit failure behavior for JSON
	// stack files that still declare autoinclude.
	if !stackConfigHasAutoIncludeHCL(stackFile) {
		return nil, nil, AutoIncludeParserStageError{
			Stage: "autoinclude-parser",
			File:  stackFilePath,
			Err: fmt.Errorf(
				"stack autoinclude is only supported for HCL stack files, not JSON: %s",
				stackFilePath,
			),
		}
	}

	// stackSrcBytes is read separately for the autoinclude parser, which slices expression byte ranges from the original file when generating terragrunt.autoinclude.hcl.
	stackSrcBytes, err := os.ReadFile(stackFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read stack file bytes %s: %w", stackFilePath, err)
	}

	// Rescope the parsing context to the stack file so terragrunt functions resolve paths relative to it instead of the root caller.
	scopedLogger, scopedPctx, scopedErr := pctx.WithConfigPath(l, stackFilePath)
	if scopedErr != nil {
		return nil, nil, AutoIncludeParserStageError{
			Stage: "rescope",
			File:  stackFilePath,
			Err:   scopedErr,
		}
	}

	// Production eval context (functions + caller variables) for the phased parser. The parser populates `local.*`, `unit.*`, `stack.*` itself.
	prodEvalCtx, evalCtxErr := createTerragruntEvalContext(
		ctx,
		scopedPctx,
		scopedLogger,
		stackFilePath,
	)
	if evalCtxErr != nil {
		return nil, nil, AutoIncludeParserStageError{
			Stage: "eval-context",
			File:  stackFilePath,
			Err:   evalCtxErr,
		}
	}

	// Evaluate the autoinclude with the stack-file function set (derived from the eval context already built
	// above) so directory-context functions like get_working_dir resolve against the stack file instead of
	// re-parsing it as a regular config.
	earlyFuncs := StackParseFunctionsFrom(prodEvalCtx.Functions, stackSourceDir)

	parseResult, parseErr := inthclparse.ParseStackFile(
		vfs.NewOSFS(),
		&inthclparse.ParseStackFileInput{
			Src:       stackSrcBytes,
			Filename:  filepath.Base(stackFilePath),
			StackDir:  stackSourceDir,
			Values:    values,
			Variables: prodEvalCtx.Variables,
			Functions: earlyFuncs,
		},
	)
	if parseErr != nil {
		return nil, nil, AutoIncludeParserStageError{
			Stage: "parse",
			File:  stackFilePath,
			Err:   parseErr,
		}
	}

	autoIncludes := parseResult.AutoIncludes

	// The phased parser resolves autoincludes from the base stack file only. A sibling
	// terragrunt.autoinclude.stack.hcl overrides same-name components wholesale, so an overridden
	// component must not inherit the base block's resolved unit-level autoinclude.
	if pruneErr := pruneOverriddenStackAutoIncludes(autoIncludes, stackSourceDir, prodEvalCtx, scopedPctx.ParserOptions); pruneErr != nil {
		return nil, nil, AutoIncludeParserStageError{
			Stage: "autoinclude-override-prune",
			File:  stackFilePath,
			Err:   pruneErr,
		}
	}

	return autoIncludes, stackSrcBytes, nil
}

// validateUpdateSourceWithCAS rejects stack files that declare update_source_with_cas = true
// on any unit or stack when CAS is not available. The attribute is meaningless without CAS
// and would otherwise silently fall through to the standard getter, producing confusing
// "source not found" failures downstream.
func validateUpdateSourceWithCAS(
	stackFile *StackConfig,
	stackFilePath string,
	casEnabled bool,
) error {
	if casEnabled {
		return nil
	}

	for _, unit := range stackFile.Units {
		if unit.UpdateSourceWithCAS != nil && *unit.UpdateSourceWithCAS {
			return &cas.UpdateSourceWithCASRequiresCASError{
				BlockType: "unit",
				Name:      unit.Name,
				Path:      stackFilePath,
			}
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack.UpdateSourceWithCAS != nil && *stack.UpdateSourceWithCAS {
			return &cas.UpdateSourceWithCASRequiresCASError{
				BlockType: "stack",
				Name:      stack.Name,
				Path:      stackFilePath,
			}
		}
	}

	return nil
}

// rejectTerraformUpdateSourceWithoutCAS rejects a generated unit whose terraform block sets
// update_source_with_cas = true when CAS is disabled. validateUpdateSourceWithCAS only sees the
// unit/stack blocks declared in the stack file; the terraform-block attribute lives in the unit's
// own terragrunt.hcl, which is only available once the source is materialized. Without this check
// the relative source would be copied verbatim and silently fail to resolve from the generated
// location, since the cas:: rewrite that gives it meaning never runs.
func rejectTerraformUpdateSourceWithoutCAS(fs vfs.FS, dest string) error {
	unitFile := filepath.Join(dest, DefaultTerragruntConfigPath)

	content, err := vfs.ReadFile(fs, unitFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to read generated unit file %s: %w", unitFile, err)
	}

	_, updateWithCAS, err := cas.ReadTerraformSourceInfo(content)
	if err != nil {
		return fmt.Errorf("failed to inspect terraform source in %s: %w", unitFile, err)
	}

	if !updateWithCAS {
		return nil
	}

	return &cas.UpdateSourceWithCASRequiresCASError{
		BlockType: "terraform",
		Path:      unitFile,
	}
}

// casSetup is the result of setupCAS: the CAS instance and Venv that
// stack/unit generation threads through every CAS call, plus the
// Enabled flag callers gate CAS features on. Enabled is false either
// because casEnabled started false or because construction failed and
// the warning was already logged.
type casSetup struct {
	Instance *cas.CAS
	Venv     cas.Venv
	Enabled  bool
}

// setupCAS prepares the CAS bundle for stack generation. A non-nil
// error is reserved for user-facing misconfiguration (invalid clone
// depth); transient setup failures log a warning and return an
// Enabled=false bundle so the caller falls through to the standard
// getter.
func setupCAS(l log.Logger, enabled bool, cloneDepth int) (casSetup, error) {
	if !enabled {
		return casSetup{}, nil
	}

	if err := cas.ValidateCASCloneDepth(cloneDepth); err != nil {
		return casSetup{}, err
	}

	c, err := cas.New(cas.WithCloneDepth(cloneDepth))
	if err != nil {
		l.Warnf("Failed to initialize CAS for stack generation: %v. CAS features disabled.", err)
		return casSetup{}, nil
	}

	v, err := cas.OSVenv()
	if err != nil {
		l.Warnf("Failed to initialize CAS environment: %v. CAS features disabled.", err)
		return casSetup{}, nil
	}

	return casSetup{Instance: c, Venv: v, Enabled: true}, nil
}

// generateOpts holds the subset of options needed for stack/unit generation.
type generateOpts struct {
	autoIncludes    map[string]*inthclparse.AutoIncludeResolved
	casInstance     *cas.CAS
	casVenv         cas.Venv
	sourceMap       map[string]string
	strictControls  strict.Controls
	rootWorkingDir  string
	stackConfigPath string
	sourceFile      string
	sourceDir       string
	targetDir       string
	stackSrcBytes   []byte
	logShowAbsPaths bool
	noStackValidate bool
	casEnabled      bool
}

// generateUnits iterates through a slice of Unit objects, generating each one by copying
// source files to their destination paths and writing unit-specific values.
// It logs the generating progress and returns any errors encountered during the operation.
func generateUnits(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	opts *generateOpts,
	pool *worker.Pool,
	units []*Unit,
) error {
	for _, unit := range units {
		pool.Submit(func() error {
			item := componentToGenerate{
				sourceDir:    opts.sourceDir,
				targetDir:    opts.targetDir,
				name:         unit.Name,
				path:         unit.Path,
				source:       unit.Source,
				values:       unit.Values,
				noStack:      unit.NoStack != nil && *unit.NoStack,
				noValidation: unit.NoValidation != nil && *unit.NoValidation,
				mutable:      unit.Mutable != nil && *unit.Mutable,
				kind:         unitKind,
			}

			l.Infof(
				"Generating unit %s from %s",
				unit.Name,
				util.RelPathForLog(opts.rootWorkingDir, opts.sourceFile, opts.logShowAbsPaths),
			)

			return telemetry.TelemeterFromContext(ctx).
				Collect(ctx, l, "stack_generate_unit", map[string]any{
					"stack_file":  opts.sourceFile,
					"unit_name":   unit.Name,
					"unit_source": unit.Source,
					"unit_path":   unit.Path,
				}, func(ctx context.Context, l log.Logger) error {
					return generateComponent(ctx, l, fs, opts, &item)
				})
		})
	}

	return nil
}

// generateStacks generates each stack by resolving its destination path and copying files from the source.
// It logs each operation and returns early if any error is encountered.
func generateStacks(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	opts *generateOpts,
	pool *worker.Pool,
	stacks []*Stack,
) error {
	for _, stack := range stacks {
		pool.Submit(func() error {
			item := componentToGenerate{
				sourceDir:    opts.sourceDir,
				targetDir:    opts.targetDir,
				name:         stack.Name,
				path:         stack.Path,
				source:       stack.Source,
				noStack:      stack.NoStack != nil && *stack.NoStack,
				noValidation: stack.NoValidation != nil && *stack.NoValidation,
				mutable:      stack.Mutable != nil && *stack.Mutable,
				values:       stack.Values,
				kind:         stackKind,
			}

			l.Infof(
				"Generating stack %s from %s",
				stack.Name,
				util.RelPathForLog(opts.rootWorkingDir, opts.sourceFile, opts.logShowAbsPaths),
			)

			return telemetry.TelemeterFromContext(ctx).
				Collect(ctx, l, "stack_generate_stack", map[string]any{
					"stack_file":   opts.sourceFile,
					"stack_name":   stack.Name,
					"stack_source": stack.Source,
					"stack_path":   stack.Path,
				}, func(ctx context.Context, l log.Logger) error {
					return generateComponent(ctx, l, fs, opts, &item)
				})
		})
	}

	return nil
}

type componentKind int

const (
	unitKind componentKind = iota
	stackKind
)

// componentToGenerate represents an item of work for generating a stack or unit.
// It contains information about the source and target directories, the name and path of the item, the source URL or path,
// and any associated values that need to be generated.
type componentToGenerate struct {
	values       *cty.Value
	sourceDir    string
	targetDir    string
	name         string
	path         string
	source       string
	noStack      bool
	noValidation bool
	mutable      bool
	kind         componentKind
}

// resolveDestPath builds and validates the destination path for a generated component.
func resolveDestPath(cmp *componentToGenerate, opts *generateOpts) (string, error) {
	if filepath.IsAbs(cmp.path) {
		return "", fmt.Errorf("path %s must be relative", cmp.path)
	}

	// Compute destination: noStack components go to parent of targetDir,
	// regular components go inside targetDir (.terragrunt-stack/).
	baseDir := opts.targetDir
	if cmp.noStack {
		baseDir = filepath.Dir(opts.targetDir)
	}

	dest := filepath.Clean(filepath.Join(baseDir, cmp.path))

	// Validate destination is within the allowed directory using filepath.Rel
	// instead of strings.HasPrefix to avoid prefix-overlap bypasses.
	rel, err := filepath.Rel(filepath.Clean(baseDir), dest)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf(
			"%s destination path '%s' is outside of the stack directory '%s'",
			cmp.name,
			dest,
			baseDir,
		)
	}

	return dest, nil
}

// validateGeneratedComponent validates the generated component directory contains the expected config file.
func validateGeneratedComponent(
	l log.Logger,
	cmp *componentToGenerate,
	opts *generateOpts,
	dest string,
) error {
	kindStr := "unit"
	if cmp.kind == stackKind {
		kindStr = "stack"
	}

	if cmp.noStack {
		l.Debugf("Skipping validation for %s %s due to no_stack flag", kindStr, cmp.name)

		return nil
	}

	if cmp.noValidation {
		l.Debugf("Skipping validation for %s %s due to no_validation flag", kindStr, cmp.name)

		return nil
	}

	expectedFile := DefaultTerragruntConfigPath

	if cmp.kind == stackKind {
		expectedFile = DefaultStackFile
	}

	if err := validateTargetDir(kindStr, cmp.name, dest, expectedFile); err != nil {
		if opts.noStackValidate {
			l.Warnf(
				"Suppressing validation error for %s %s at path %s: expected %s to generate with %s file at root of generated directory.",
				kindStr,
				cmp.name,
				opts.targetDir,
				kindStr,
				expectedFile,
			)

			return nil
		}

		return fmt.Errorf(
			"validation failed for %s %s at path %s: expected %s to generate with %s file at root of generated directory",
			kindStr,
			cmp.name,
			opts.targetDir,
			kindStr,
			expectedFile,
		)
	}

	return nil
}

// generateAutoInclude writes the autoinclude file for a component if one was resolved.
func generateAutoInclude(
	l log.Logger,
	fs vfs.FS,
	opts *generateOpts,
	cmp *componentToGenerate,
	dest string,
) error {
	if opts.autoIncludes == nil {
		return nil
	}

	kind := inthclparse.KindUnit
	if cmp.kind == stackKind {
		kind = inthclparse.KindStack
	}

	resolved, ok := opts.autoIncludes[inthclparse.AutoIncludeKey(kind, cmp.name)]
	if !ok {
		return nil
	}

	l.Infof(
		"Generating %s for %s %s in %s",
		inthclparse.AutoIncludeFileNameForKind(kind),
		kind,
		cmp.name,
		util.RelPathForLog(opts.rootWorkingDir, dest, opts.logShowAbsPaths),
	)

	// The autoinclude resolves entirely in the stack file context, so the resolution-time eval context (functions
	// scoped to the stack file, like the discovery path) is reused as-is: every expression except dependency.* is
	// already a literal, and directory-context functions resolve where the autoinclude was authored.
	if err := inthclparse.GenerateAutoIncludeFile(fs, resolved, dest, resolved.SourceBytes, resolved.EvalCtx); err != nil {
		return fmt.Errorf("failed to write autoinclude for %s %s: %w", kind, cmp.name, err)
	}

	return nil
}

// generateComponent copies files from the source directory to the target destination and generates a corresponding values file.
func generateComponent(
	ctx context.Context,
	l log.Logger,
	fs vfs.FS,
	opts *generateOpts,
	cmp *componentToGenerate,
) error {
	source := cmp.source
	// Adjust source path using the provided source mapping configuration if available
	source, err := adjustSourceWithMap(opts.sourceMap, source, opts.stackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to adjust source %s: %w", cmp.source, err)
	}

	dest, err := resolveDestPath(cmp, opts)
	if err != nil {
		return err
	}

	kindStr := "unit"
	if cmp.kind == stackKind {
		kindStr = "stack"
	}

	l.Debugf("Generating: %s (%s) to %s", cmp.name, source, dest)

	if err := fetchComponentSource(ctx, l, opts, cmp, kindStr, source, dest); err != nil {
		return err
	}

	if !opts.casEnabled && cmp.kind == unitKind {
		if err := rejectTerraformUpdateSourceWithoutCAS(fs, dest); err != nil {
			return err
		}
	}

	if err := validateGeneratedComponent(l, cmp, opts, dest); err != nil {
		return err
	}

	if err := writeValues(l, cmp.values, dest); err != nil {
		return fmt.Errorf("failed to write values %v %w", cmp.name, err)
	}

	return generateAutoInclude(l, fs, opts, cmp, dest)
}

// fetchComponentSource handles the paths for fetching a component's source:
//  1. cas:: protocol source: materialize the CAS tree directly. Must run before the CAS-backed
//     branch so nested components already rewritten to cas:: are not re-fetched.
//  2. Source with CAS enabled: attempt a CAS-backed fetch (remote clone or local copy into a
//     temp overlay) and copy from its content dir. This fires automatically whenever the cas
//     experiment is on, so catalog authors don't need consumers to opt in per-block. On most CAS
//     failures (for example, CAS can't handle a go-getter query param like depth=1), we fall
//     through to the standard getter. Configuration errors that can never succeed via CAS, such
//     as an interpolated source on a block with update_source_with_cas = true, fail generation
//     instead of falling through.
//  3. Standard: local copy or remote getter.
//
// The update_source_with_cas attribute on a consumer block is a no-op under path 2. It only
// matters inside catalog files, where the CAS walk uses it to decide which nested sources to
// rewrite to cas:: references.
func fetchComponentSource(
	ctx context.Context,
	l log.Logger,
	opts *generateOpts,
	cmp *componentToGenerate,
	kindStr, source, dest string,
) error {
	source = tf.RewriteLegacyGCSPublicSource(ctx, l, source, opts.strictControls)

	if isCASProtocol(source) {
		if !opts.casEnabled {
			return fmt.Errorf(
				"cas:: source on %s %q requires the --no-cas flag to be unset",
				kindStr,
				cmp.name,
			)
		}

		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory %s for %s %w", dest, cmp.name, err)
		}

		// Strip the cas:: prefix and parse the hash
		casRef := strings.TrimPrefix(source, cas.CASProtocolPrefix)

		hash, err := cas.ParseCASRef(casRef)
		if err != nil {
			return fmt.Errorf("failed to parse CAS reference for %s %s: %w", kindStr, cmp.name, err)
		}

		var matOpts []cas.LinkTreeOption
		if cmp.mutable {
			matOpts = append(matOpts, cas.WithForceCopy())
		}

		if err := opts.casInstance.MaterializeTree(ctx, l, opts.casVenv, hash, dest, matOpts...); err != nil {
			return fmt.Errorf(
				"failed to materialize CAS tree for %s %s: %w",
				kindStr,
				cmp.name,
				err,
			)
		}

		return nil
	}

	if opts.casEnabled {
		casErr := fetchViaCAS(ctx, l, opts, cmp.sourceDir, kindStr, source, dest)
		if casErr == nil {
			return nil
		}

		// A non-literal source on an update_source_with_cas block can never
		// be rewritten by CAS, so falling back would silently skip the rewrite
		// the configuration asked for. Surface the error instead.
		if errors.Is(casErr, cas.ErrSourceNotLiteral) {
			return fmt.Errorf("failed to fetch %s %q via CAS: %w", kindStr, cmp.name, casErr)
		}

		l.Warnf(
			"CAS processing failed for %s %q: %v. Falling back to standard getter.",
			kindStr,
			cmp.name,
			casErr,
		)
		cas.RecordFallback(ctx, l, cas.FallbackReasonStackGenerationError, map[string]any{
			"kind":   kindStr,
			"name":   cmp.name,
			"source": source,
		})
	}

	if err := copyFiles(ctx, l, cmp.name, cmp.sourceDir, source, dest); err != nil {
		return fmt.Errorf(
			"failed to fetch %s %s\n"+
				"  Source:      %s\n"+
				"  Destination: %s\n\n"+
				"Troubleshooting:\n"+
				"  1. Check if your source path is correct relative to the stack file location\n"+
				"  2. Verify the units or stacks directory exists at the expected location\n"+
				"  3. Ensure you have proper permissions to read from source and write to destination\n\n"+
				"Original error: %w",
			kindStr,
			cmp.name,
			source,
			dest,
			err,
		)
	}

	return nil
}

// fetchViaCAS performs the CAS-backed fetch, rewrite, and copy for a stack component.
// For remote sources the fetch is a CAS-backed git clone; for local sources it is a
// copy into a temp overlay so rewrites do not mutate the caller's working tree.
func fetchViaCAS(
	ctx context.Context,
	l log.Logger,
	opts *generateOpts,
	sourceDir, kindStr, source, dest string,
) error {
	resolvedSource := resolveLocalCASSource(l, sourceDir, source)

	result, err := opts.casInstance.ProcessStackComponent(
		ctx,
		l,
		opts.casVenv,
		resolvedSource,
		kindStr,
	)
	if err != nil {
		return err
	}

	defer result.Cleanup()

	// A copy failure can leave partial content in dest, so reset it before
	// falling through to the standard getter. ProcessStackComponent writes only
	// to its own temp dir, so failures before this point never touch dest.
	if copyErr := util.CopyFolderContentsWithFilter(l, result.ContentDir, dest, manifestName, func(_ string) bool {
		return true
	}); copyErr != nil {
		if cleanupErr := os.RemoveAll(dest); cleanupErr != nil &&
			!errors.Is(cleanupErr, os.ErrNotExist) {
			l.Debugf("Failed to clean partial CAS destination %s: %v", dest, cleanupErr)
		}

		return copyErr
	}

	return nil
}

// resolveLocalCASSource normalizes a relative local source against sourceDir so
// ProcessStackComponent's filepath.Abs/Stat resolve against the stack file
// rather than the process CWD. Remote sources, absolute paths, and any source
// that doesn't look like a local path are returned unchanged. The "//" subdir
// suffix used by go-getter is preserved.
func resolveLocalCASSource(l log.Logger, sourceDir, source string) string {
	if source == "" || sourceDir == "" {
		return source
	}

	basePath, subdir := getter.SourceDirSubdir(source)
	if filepath.IsAbs(basePath) || !isLocal(l, sourceDir, basePath) {
		return source
	}

	abs, err := filepath.Abs(filepath.Join(sourceDir, basePath))
	if err != nil {
		return source
	}

	if subdir != "" {
		return abs + "//" + subdir
	}

	return abs
}

// isCASProtocol checks if a source string uses the CAS protocol (cas::sha1:<hash>).
func isCASProtocol(source string) bool {
	return strings.HasPrefix(source, cas.CASProtocolPrefix)
}

// copyFiles copies files or directories from a source to a destination path.
//
// The function checks if the source is local or remote. If local, it copies the
// contents of the source directory to the destination. If remote, it fetches the
// source and stores it in the destination directory.
func copyFiles(ctx context.Context, l log.Logger, identifier, sourceDir, src, dest string) error {
	if !isLocal(l, sourceDir, src) {
		if err := os.MkdirAll(dest, os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory %s for %s %w", dest, identifier, err)
		}

		if _, err := getter.GetAny(ctx, dest, src); err != nil {
			return fmt.Errorf("failed to fetch %s %s for %s %w", src, dest, identifier, err)
		}

		return nil
	}

	localSrc := src

	if !filepath.IsAbs(src) {
		localSrc = filepath.Join(sourceDir, src)
	}

	localSrc = filepath.Clean(localSrc)

	if err := util.CopyFolderContentsWithFilter(l, localSrc, dest, manifestName, func(absolutePath string) bool {
		return true
	}); err != nil {
		return fmt.Errorf("failed to copy %s to %s %w", localSrc, dest, err)
	}

	return nil
}

// isLocal determines if a given source path is local or remote.
//
// It checks if the provided source file exists locally, or relative to the
// working directory, or carries an explicit file:// scheme. A source that
// looks like an absolute path but does not exist is treated as remote so the
// caller can produce a meaningful fetch error rather than silently copying
// from an empty directory.
func isLocal(_ log.Logger, workingDir, src string) bool {
	if util.FileExists(src) {
		return true
	}

	if util.FileExists(filepath.Join(workingDir, src)) {
		return true
	}

	return strings.HasPrefix(src, "file://")
}

// ReadOutputs retrieves the OpenTofu/Terraform output JSON for this unit, converts it into a map of cty.Values,
// and logs the operation for debugging. It returns early in case of any errors during retrieval or conversion.
func (u *Unit) ReadOutputs(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	unitDir string,
) (map[string]cty.Value, error) {
	configPath := filepath.Join(unitDir, DefaultTerragruntConfigPath)
	l.Debugf("Getting output from unit %s in %s", u.Name, unitDir)

	jsonBytes, err := getOutputJSONWithCaching(ctx, pctx, l, configPath)
	if err != nil {
		return nil, err
	}

	outputMap, err := TerraformOutputJSONToCtyValueMap(configPath, jsonBytes)
	if err != nil {
		return nil, err
	}

	return outputMap, nil
}

// ReadStackConfigFile reads and parses a Terragrunt stack configuration file from the given path.
// It creates a parsing context, processes locals, and decodes the file into a StackConfig struct.
// Validation is performed on the resulting config, and any encountered errors cause an early return.
func ReadStackConfigFile(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	filePath string,
	values *cty.Value,
) (*StackConfig, error) {
	l.Debugf("Reading Terragrunt stack config file at %s", filePath)

	stackPctx := pctx.Clone()
	stackPctx.TerragruntConfigPath = filePath
	stackPctx.OriginalTerragruntConfigPath = filePath

	file, err := hclparse.NewParser(stackPctx.ParserOptions...).ParseFromFile(filePath)
	if err != nil {
		return nil, err
	}

	return ParseStackConfig(ctx, l, stackPctx, file, values)
}

// ReadStackConfigString reads and parses a Terragrunt stack configuration from a string.
func ReadStackConfigString(
	ctx context.Context,
	l log.Logger,
	pctx *ParsingContext,
	configPath string,
	configString string,
	values *cty.Value,
) (*StackConfig, error) {
	if values != nil {
		pctx = pctx.WithValues(values)
	}

	hclFile, err := hclparse.NewParser(pctx.ParserOptions...).
		ParseFromString(configString, configPath)
	if err != nil {
		return nil, err
	}

	return ParseStackConfig(ctx, l, pctx, hclFile, values)
}

// ParseStackConfig parses the stack configuration from the given file and values.
func ParseStackConfig(
	ctx context.Context,
	l log.Logger,
	parser *ParsingContext,
	file *hclparse.File,
	values *cty.Value,
) (*StackConfig, error) {
	if values != nil {
		parser = parser.WithValues(values)
	}

	if err := processLocals(ctx, l, parser, file); err != nil {
		return nil, err
	}

	evalParsingContext, err := createTerragruntEvalContext(ctx, parser, l, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	// Expose unit.<name>.path / stack.<name>.path so a unit or stack block's values
	// can reference where sibling components generate to (e.g. to pass a unit path
	// down to a child stack).
	if err := injectStackComponentRefs(file, evalParsingContext, filepath.Dir(file.ConfigPath), parser.ParserOptions); err != nil {
		return nil, err
	}

	config := &StackConfigFile{}
	if decodeErr := file.Decode(config, evalParsingContext); decodeErr != nil {
		return nil, decodeErr
	}

	// Process include blocks and merge any generated stack-level autoinclude file.
	stackDir := filepath.Dir(file.ConfigPath)

	if err := processStackConfigIncludes(config, stackDir, evalParsingContext, parser.ParserOptions); err != nil {
		return nil, err
	}

	if err := mergeStackAutoIncludeFile(l, config, stackDir, filepath.Base(file.ConfigPath), evalParsingContext, parser.ParserOptions); err != nil {
		return nil, err
	}

	localsParsed := map[string]any{}

	if parser.Locals != nil {
		var err error

		localsParsed, err = ctyhelper.ParseCtyValueToMap(*parser.Locals)
		if err != nil {
			return nil, err
		}
	}

	stackConfig := &StackConfig{
		Locals: localsParsed,
		Stacks: config.Stacks,
		Units:  config.Units,
	}

	if err := ValidateStackConfig(config, filepath.Dir(file.ConfigPath)); err != nil {
		return nil, err
	}

	return stackConfig, nil
}

// stackComponentHeaders captures only the label and path of each unit/stack block
// so component paths can be resolved before the full decode evaluates values.
// source, values, and every other attribute are left in the block body, unevaluated.
type stackComponentHeaders struct {
	Remain hcl.Body                `hcl:",remain"`
	Stacks []*stackComponentHeader `hcl:"stack,block"`
	Units  []*stackComponentHeader `hcl:"unit,block"`
}

// stackComponentHeader is the path-only shape of a unit or stack block.
type stackComponentHeader struct {
	Remain  hcl.Body `hcl:",remain"`
	NoStack *bool    `hcl:"no_dot_terragrunt_stack,optional"`
	Path    string   `hcl:"path,attr"`
	Name    string   `hcl:",label"`
}

// GeneratedPath returns the on-disk path this component generates to under stackDir.
func (h *stackComponentHeader) GeneratedPath(stackDir string) string {
	return inthclparse.GeneratedComponentPath(stackDir, h.Path, h.NoStack != nil && *h.NoStack)
}

// injectStackComponentRefs adds the unit.<name> and stack.<name> path variables to
// evalCtx so a unit/stack block's values can reference where sibling components
// generate to. It decodes only block labels and paths first, leaving source and
// values unevaluated, so it can run before the value that depends on these refs is
// evaluated. A sibling terragrunt.autoinclude.stack.hcl is folded by name so an
// overridden component's path reflects the override, not the stale base path.
// stackDir is the directory containing the stack file.
func injectStackComponentRefs(
	file *hclparse.File,
	evalCtx *hcl.EvalContext,
	stackDir string,
	parserOpts []hclparse.Option,
) error {
	headers := &stackComponentHeaders{}
	if err := file.Decode(headers, evalCtx); err != nil {
		return err
	}

	// Publish the base refs first so a sibling autoinclude block whose path references unit.<name>.path /
	// stack.<name>.path can resolve against the base components, matching how the full decode resolves them.
	setStackComponentRefVars(evalCtx, stackDir, headers.Units, headers.Stacks)

	autoUnits, autoStacks, err := stackAutoIncludeComponentHeaders(stackDir, evalCtx, parserOpts)
	if err != nil {
		return err
	}

	// Republish so an overridden component's path reflects the override, not the base path it replaced.
	units := util.MergeNamed(headers.Units, autoUnits, componentHeaderName)
	stacks := util.MergeNamed(headers.Stacks, autoStacks, componentHeaderName)
	setStackComponentRefVars(evalCtx, stackDir, units, stacks)

	return nil
}

// setStackComponentRefVars publishes the unit.<name> and stack.<name> path variables into evalCtx.
func setStackComponentRefVars(
	evalCtx *hcl.EvalContext,
	stackDir string,
	units, stacks []*stackComponentHeader,
) {
	unitRefs := make([]inthclparse.ComponentRef, 0, len(units))

	for _, u := range units {
		if u == nil {
			continue
		}

		unitRefs = append(
			unitRefs,
			inthclparse.ComponentRef{Name: u.Name, Path: u.GeneratedPath(stackDir)},
		)
	}

	stackRefs := make([]inthclparse.ComponentRef, 0, len(stacks))

	for _, s := range stacks {
		if s == nil {
			continue
		}

		stackRefs = append(
			stackRefs,
			inthclparse.ComponentRef{Name: s.Name, Path: s.GeneratedPath(stackDir)},
		)
	}

	evalCtx.Variables[inthclparse.VarUnit] = inthclparse.BuildComponentRefMap(unitRefs)
	evalCtx.Variables[inthclparse.VarStack] = inthclparse.BuildComponentRefMap(stackRefs)
}

// stackAutoIncludeComponentHeaders decodes the unit and stack block headers (name and path only) declared
// by a sibling terragrunt.autoinclude.stack.hcl. It returns nil slices when no autoinclude file exists.
func stackAutoIncludeComponentHeaders(
	stackDir string,
	evalCtx *hcl.EvalContext,
	parserOpts []hclparse.Option,
) ([]*stackComponentHeader, []*stackComponentHeader, error) {
	autoIncludePath := filepath.Join(stackDir, inthclparse.AutoIncludeStackFile)
	if !util.FileExists(autoIncludePath) {
		return nil, nil, nil
	}

	incFile, err := hclparse.NewParser(parserOpts...).ParseFromFile(autoIncludePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read stack autoinclude %q: %w", autoIncludePath, err)
	}

	headers := &stackComponentHeaders{}
	if decodeErr := incFile.Decode(headers, evalCtx); decodeErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to decode stack autoinclude headers %q: %w",
			autoIncludePath,
			decodeErr,
		)
	}

	return headers.Units, headers.Stacks, nil
}

// componentHeaderName returns a header's block name, or an empty string for a nil entry so MergeNamed leaves it untouched.
func componentHeaderName(h *stackComponentHeader) string {
	if h == nil {
		return ""
	}

	return h.Name
}

// stackComponentLabel captures only a unit or stack block label, leaving every attribute (including path)
// in Remain so the block name can be read without evaluating any expression.
type stackComponentLabel struct {
	Remain hcl.Body `hcl:",remain"`
	Name   string   `hcl:",label"`
}

// stackComponentLabels is the label-only shape of a stack file's unit and stack blocks.
type stackComponentLabels struct {
	Remain hcl.Body               `hcl:",remain"`
	Stacks []*stackComponentLabel `hcl:"stack,block"`
	Units  []*stackComponentLabel `hcl:"unit,block"`
}

// stackAutoIncludeComponentNames returns the unit and stack block names declared by a sibling
// terragrunt.autoinclude.stack.hcl without evaluating their path expressions, so callers that only need
// names do not depend on local.*/unit.*/stack.* being populated in the eval context. It returns nil slices
// when no autoinclude file exists.
func stackAutoIncludeComponentNames(
	stackDir string,
	evalCtx *hcl.EvalContext,
	parserOpts []hclparse.Option,
) (unitNames, stackNames []string, err error) {
	autoIncludePath := filepath.Join(stackDir, inthclparse.AutoIncludeStackFile)
	if !util.FileExists(autoIncludePath) {
		return nil, nil, nil
	}

	incFile, err := hclparse.NewParser(parserOpts...).ParseFromFile(autoIncludePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read stack autoinclude %q: %w", autoIncludePath, err)
	}

	labels := &stackComponentLabels{}
	if decodeErr := incFile.Decode(labels, evalCtx); decodeErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to decode stack autoinclude labels %q: %w",
			autoIncludePath,
			decodeErr,
		)
	}

	for _, u := range labels.Units {
		if u == nil {
			continue
		}

		unitNames = append(unitNames, u.Name)
	}

	for _, s := range labels.Stacks {
		if s == nil {
			continue
		}

		stackNames = append(stackNames, s.Name)
	}

	return unitNames, stackNames, nil
}

// pruneOverriddenStackAutoIncludes drops the base-resolved unit-level autoinclude for any component the
// sibling terragrunt.autoinclude.stack.hcl overrides by name, so an overridden component does not inherit
// the base block's autoinclude (the override is wholesale). A newly injected name has no base entry, so
// pruning it is a no-op. It reads only block names so it never evaluates an injected path expression that
// the generate-path eval context cannot resolve.
func pruneOverriddenStackAutoIncludes(
	autoIncludes map[string]*inthclparse.AutoIncludeResolved,
	stackDir string,
	evalCtx *hcl.EvalContext,
	parserOpts []hclparse.Option,
) error {
	if len(autoIncludes) == 0 {
		return nil
	}

	unitNames, stackNames, err := stackAutoIncludeComponentNames(stackDir, evalCtx, parserOpts)
	if err != nil {
		return err
	}

	for _, name := range unitNames {
		delete(autoIncludes, inthclparse.AutoIncludeKey(inthclparse.KindUnit, name))
	}

	for _, name := range stackNames {
		delete(autoIncludes, inthclparse.AutoIncludeKey(inthclparse.KindStack, name))
	}

	return nil
}

// processStackConfigIncludes resolves include blocks during stack file parsing.
// It reads each included file, parses it with the same eval context, and merges
// its units and stacks into the main config so generation sees all components,
// not just those in the root file.
func processStackConfigIncludes(
	config *StackConfigFile,
	stackDir string,
	evalCtx *hcl.EvalContext,
	parserOpts []hclparse.Option,
) error {
	for _, inc := range config.Includes {
		includePath := inc.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(stackDir, includePath)
		}

		incFile, err := hclparse.NewParser(parserOpts...).ParseFromFile(includePath)
		if err != nil {
			return fmt.Errorf("failed to read include %q: %w", inc.Name, err)
		}

		included := &StackConfigFile{}
		if decodeErr := incFile.Decode(included, evalCtx); decodeErr != nil {
			return fmt.Errorf("failed to decode include %q: %w", inc.Name, decodeErr)
		}

		if included.Locals != nil {
			return fmt.Errorf("included stack file %q must not define locals", inc.Name)
		}

		if len(included.Includes) > 0 {
			return fmt.Errorf("included stack file %q must not define nested includes", inc.Name)
		}

		config.Units = append(config.Units, included.Units...)
		config.Stacks = append(config.Stacks, included.Stacks...)
	}

	// Validate no duplicate unit names after merge.
	seen := make(map[string]struct{}, len(config.Units))

	for _, u := range config.Units {
		if _, exists := seen[u.Name]; exists {
			return inthclparse.DuplicateUnitNameError{Name: u.Name}
		}

		seen[u.Name] = struct{}{}
	}

	// Validate no duplicate stack names after merge.
	seen = make(map[string]struct{}, len(config.Stacks))

	for _, s := range config.Stacks {
		if _, exists := seen[s.Name]; exists {
			return inthclparse.DuplicateStackNameError{Name: s.Name}
		}

		seen[s.Name] = struct{}{}
	}

	return nil
}

// mergeStackAutoIncludeFile merges a generated terragrunt.autoinclude.stack.hcl, if present
// beside the stack file, into the stack config. Units and stacks injected by a parent stack's
// autoinclude block materialize in the nested stack the same way a unit's
// terragrunt.autoinclude.hcl merges into its terragrunt.hcl via [mergeAutoIncludeIfPresent].
func mergeStackAutoIncludeFile(
	l log.Logger,
	config *StackConfigFile,
	stackDir, stackFileName string,
	evalCtx *hcl.EvalContext,
	parserOpts []hclparse.Option,
) error {
	// Never merge the autoinclude file into itself.
	if stackFileName == inthclparse.AutoIncludeStackFile {
		return nil
	}

	autoIncludePath := filepath.Join(stackDir, inthclparse.AutoIncludeStackFile)
	if !util.FileExists(autoIncludePath) {
		return nil
	}

	incFile, err := hclparse.NewParser(parserOpts...).ParseFromFile(autoIncludePath)
	if err != nil {
		return fmt.Errorf("failed to read stack autoinclude %q: %w", autoIncludePath, err)
	}

	// In production the file is parsed with hclsyntax, so the body is always *hclsyntax.Body; surface the impossible-state assertion rather than silently skipping the backstop.
	syntaxBody, ok := incFile.Body.(*hclsyntax.Body)
	if !ok {
		return inthclparse.UnexpectedBodyTypeError{FilePath: autoIncludePath}
	}

	// Backstop the fail-fast generation check for stale or hand-written files using the shared scan.
	if typed := inthclparse.StackAutoIncludeDepValuesError(syntaxBody, filepath.Base(stackDir)); typed != nil {
		return *typed
	}

	included := &StackConfigFile{}
	if decodeErr := incFile.Decode(included, evalCtx); decodeErr != nil {
		return fmt.Errorf("failed to decode stack autoinclude %q: %w", autoIncludePath, decodeErr)
	}

	if included.Locals != nil {
		return fmt.Errorf("stack autoinclude %q must not define locals", autoIncludePath)
	}

	if len(included.Includes) > 0 {
		return fmt.Errorf("stack autoinclude %q must not define include blocks", autoIncludePath)
	}

	// Reject duplicate names within the autoinclude file itself, mirroring the base-file rejection, so a
	// stale or hand-edited autoinclude cannot silently collapse two same-name blocks into one.
	if err := validateUniqueComponentNames(included.Units, included.Stacks); err != nil {
		return err
	}

	logStackAutoIncludeMergeNotes(l, config, included)

	// A same-name injected unit/stack overrides the base block wholesale, matching unit autoinclude override semantics.
	config.Units = util.MergeNamed(config.Units, included.Units, unitName)
	config.Stacks = util.MergeNamed(config.Stacks, included.Stacks, stackName)

	return nil
}

// validateUniqueComponentNames reports the first duplicate unit or stack name in the given slices as the
// same typed error the base stack file raises, so an autoinclude file cannot silently collapse same-name blocks.
func validateUniqueComponentNames(units []*Unit, stacks []*Stack) error {
	seenUnits := make(map[string]struct{}, len(units))

	for _, u := range units {
		if u == nil {
			continue
		}

		if _, dup := seenUnits[u.Name]; dup {
			return inthclparse.DuplicateUnitNameError{Name: u.Name}
		}

		seenUnits[u.Name] = struct{}{}
	}

	seenStacks := make(map[string]struct{}, len(stacks))

	for _, s := range stacks {
		if s == nil {
			continue
		}

		if _, dup := seenStacks[s.Name]; dup {
			return inthclparse.DuplicateStackNameError{Name: s.Name}
		}

		seenStacks[s.Name] = struct{}{}
	}

	return nil
}

// unitName returns a unit's block name, or an empty string for a nil entry so MergeNamed leaves it untouched.
func unitName(u *Unit) string {
	if u == nil {
		return ""
	}

	return u.Name
}

// stackName returns a stack's block name, or an empty string for a nil entry so MergeNamed leaves it untouched.
func stackName(s *Stack) string {
	if s == nil {
		return ""
	}

	return s.Name
}

// writeValues generates and writes values to a terragrunt.values.hcl file in the specified directory.
func writeValues(l log.Logger, values *cty.Value, directory string) error {
	if values == nil {
		l.Debugf("No values to write in %s", directory)
		return nil
	}
	// Avoid panics if the provided values are in unsupported format
	if values.IsNull() {
		l.Debugf("Skipping writing values in %s: values is null", directory)
		return nil
	}

	if !values.IsWhollyKnown() {
		l.Debugf("Skipping writing values in %s: values are not fully known", directory)
		return nil
	}

	valType := values.Type()

	if !valType.IsObjectType() && !valType.IsMapType() {
		return fmt.Errorf("writeValues: expected object or map, got %s", valType.FriendlyName())
	}

	if directory == "" {
		return errors.New("writeValues: unit directory path cannot be empty")
	}

	if err := os.MkdirAll(directory, unitDirPerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", directory, err)
	}

	l.Debugf("Writing values file in %s", directory)
	filePath := filepath.Join(directory, valuesFile)

	file := hclwrite.NewEmptyFile()
	body := file.Body()
	body.AppendUnstructuredTokens([]*hclwrite.Token{
		{
			Type: hclsyntax.TokenComment,
			Bytes: []byte(
				"# Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually\n",
			),
		},
	})

	// Sort keys for deterministic output
	valueMap := values.AsValueMap()

	keys := make([]string, 0, len(valueMap))
	for key := range valueMap {
		keys = append(keys, key)
	}

	// Sort keys alphabetically
	sort.Strings(keys)

	for _, key := range keys {
		body.SetAttributeValue(key, valueMap[key])
	}

	// CAS may materialize the target as a read-only hard link, so remove it before writing
	if util.IsFile(filePath) {
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove values file %s before writing: %w", filePath, err)
		}
	}

	if err := os.WriteFile(filePath, file.Bytes(), valueFilePerm); err != nil {
		return fmt.Errorf("failed to write values file %s: %w", filePath, err)
	}

	return nil
}

// ReadValues reads values from the terragrunt.values.hcl file in the specified directory.
func ReadValues(
	ctx context.Context,
	pctx *ParsingContext,
	l log.Logger,
	directory string,
) (*cty.Value, error) {
	if directory == "" {
		return nil, errors.New("ReadValues: directory path cannot be empty")
	}

	filePath := filepath.Join(directory, valuesFile)

	if util.FileNotExists(filePath) {
		return nil, nil
	}

	l.Debugf("Reading Terragrunt stack values file at %s", filePath)

	file, err := hclparse.NewParser(pctx.ParserOptions...).ParseFromFile(filePath)
	if err != nil {
		return nil, err
	}

	evalParsingContext, err := createTerragruntEvalContext(ctx, pctx, l, file.ConfigPath)
	if err != nil {
		return nil, err
	}

	values := map[string]cty.Value{}

	if err := file.Decode(&values, evalParsingContext); err != nil {
		return nil, err
	}

	result := cty.ObjectVal(values)

	return &result, nil
}

// processLocals processes the locals block in the stack file.
func processLocals(
	ctx context.Context,
	l log.Logger,
	parser *ParsingContext,
	file *hclparse.File,
) error {
	localsBlock, err := file.Blocks(MetadataLocals, false)
	if err != nil {
		return err
	}

	if len(localsBlock) == 0 {
		return nil
	}

	if len(localsBlock) > 1 {
		return fmt.Errorf(
			"up to one locals block is allowed per stack file, but found %d in %s",
			len(localsBlock),
			file.ConfigPath,
		)
	}

	attrs, err := localsBlock[0].JustAttributes()
	if err != nil {
		return err
	}

	evaluatedLocals := map[string]cty.Value{}
	evaluated := true

	for iterations := 0; len(attrs) > 0 && evaluated; iterations++ {
		if iterations > MaxIter {
			// Reached maximum supported iterations, which is most likely an infinite loop bug so cut the iteration
			// short and return an error.
			return MaxIterError{}
		}

		var evalErr error

		attrs, evaluatedLocals, evaluated, evalErr = attemptEvaluateLocals(
			ctx,
			parser,
			l,
			file,
			attrs,
			evaluatedLocals,
		)
		if evalErr != nil {
			l.Debugf(
				"Encountered error while evaluating locals in file %s",
				util.RelPathForLog(parser.RootWorkingDir, file.ConfigPath, parser.LogShowAbsPaths),
			)

			return evalErr
		}
	}

	localsAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedLocals)
	if err != nil {
		return err
	}

	parser.Locals = &localsAsCtyVal

	return nil
}

// validateTargetDir target destination directory.
func validateTargetDir(kind, name, destDir, expectedFile string) error {
	expectedPath := filepath.Join(destDir, expectedFile)

	info, err := os.Stat(expectedPath)
	if err != nil {
		return fmt.Errorf(
			"%s '%s': expected file '%s' not found in target directory '%s': %w",
			kind,
			name,
			expectedFile,
			destDir,
			err,
		)
	}

	if info.IsDir() {
		return fmt.Errorf(
			"%s '%s': expected file '%s' is a directory, not a file",
			kind,
			name,
			expectedFile,
		)
	}

	return nil
}

// stackConfigHasAutoInclude reports whether any unit or stack in the include-merged stack config declares an autoinclude block.
func stackConfigHasAutoInclude(stackFile *StackConfig) bool {
	if stackFile == nil {
		return false
	}

	for _, unit := range stackFile.Units {
		if unit != nil && bodyHasBlock(unit.Remain) {
			return true
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack != nil && bodyHasBlock(stack.Remain) {
			return true
		}
	}

	return false
}

func stackConfigHasAutoIncludeHCL(stackFile *StackConfig) bool {
	if stackFile == nil {
		return false
	}

	for _, unit := range stackFile.Units {
		if unit == nil {
			continue
		}

		syntaxBody, ok := unit.Remain.(*hclsyntax.Body)
		if !ok {
			continue
		}

		if bodyHasBlock(syntaxBody) {
			return true
		}
	}

	for _, stack := range stackFile.Stacks {
		if stack == nil {
			continue
		}

		syntaxBody, ok := stack.Remain.(*hclsyntax.Body)
		if !ok {
			continue
		}

		if bodyHasBlock(syntaxBody) {
			return true
		}
	}

	return false
}

// bodyHasBlock reports whether body contains a top-level autoinclude block.
// Works on both native HCL and JSON-format bodies via the schema-driven PartialContent API.
// Intentional fail-open: on any PartialContent diagnostic we return true so the
// parser still runs and surfaces the real parse diagnostic.
func bodyHasBlock(body hcl.Body) bool {
	if body == nil {
		return false
	}

	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "autoinclude"}},
	})
	if diags.HasErrors() {
		return true
	}

	return len(content.Blocks) > 0
}

// logStackAutoIncludeMergeNotes records when an injected unit/stack name overrides an existing one and when a nested autoinclude block is dropped. A same-name injected block replaces the base block wholesale, matching unit autoinclude override semantics.
func logStackAutoIncludeMergeNotes(l log.Logger, config, included *StackConfigFile) {
	existingUnits := unitNameSet(config.Units)
	existingStacks := stackNameSet(config.Stacks)

	for _, unit := range included.Units {
		if unit == nil {
			continue
		}

		if _, clash := existingUnits[unit.Name]; clash {
			l.Debugf(
				"Stack autoinclude unit %q overrides the same-name unit in the target stack config",
				unit.Name,
			)
		}

		if bodyHasBlock(unit.Remain) {
			l.Debugf(
				"Stack autoinclude unit %q declares a nested autoinclude block; nested autoinclude is not propagated into the injected component",
				unit.Name,
			)
		}
	}

	for _, stack := range included.Stacks {
		if stack == nil {
			continue
		}

		if _, clash := existingStacks[stack.Name]; clash {
			l.Debugf(
				"Stack autoinclude stack %q overrides the same-name stack in the target stack config",
				stack.Name,
			)
		}

		if bodyHasBlock(stack.Remain) {
			l.Debugf(
				"Stack autoinclude stack %q declares a nested autoinclude block; nested autoinclude is not propagated into the injected component",
				stack.Name,
			)
		}
	}
}

// unitNameSet returns the set of unit names.
func unitNameSet(units []*Unit) map[string]struct{} {
	names := make(map[string]struct{}, len(units))

	for _, unit := range units {
		if unit == nil {
			continue
		}

		names[unit.Name] = struct{}{}
	}

	return names
}

// stackNameSet returns the set of stack names.
func stackNameSet(stacks []*Stack) map[string]struct{} {
	names := make(map[string]struct{}, len(stacks))

	for _, stack := range stacks {
		if stack == nil {
			continue
		}

		names[stack.Name] = struct{}{}
	}

	return names
}
