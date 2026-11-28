package config

import (
	"context"
	"fmt"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"errors"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// PartialDecodeSectionType is an enum that is used to list out which blocks/sections of the terragrunt config should be
// decoded in a partial decode.
type PartialDecodeSectionType int

const (
	// DependenciesBlock decodes the `dependencies` block.
	DependenciesBlock PartialDecodeSectionType = iota
	// DependencyBlock decodes the `dependency` blocks and adds their config paths to the config's dependencies.
	DependencyBlock
	// TerraformBlock decodes the whole `terraform` block.
	TerraformBlock
	// TerraformSource decodes only the `source` attribute of the `terraform` block.
	TerraformSource
	// TerragruntFlags decodes the `prevent_destroy`, `iam_role`, and `iam_web_identity_token` attributes.
	TerragruntFlags
	// TerragruntVersionConstraints decodes the `terragrunt_version_constraint`, `terraform_version_constraint`, and
	// `terraform_binary` attributes.
	TerragruntVersionConstraints
	// RemoteStateBlock decodes the `remote_state` block or attribute.
	RemoteStateBlock
	// FeatureFlagsBlock decodes the `feature` blocks.
	FeatureFlagsBlock
	// EngineBlock decodes the `engine` block.
	EngineBlock
	// ExcludeBlock evaluates the `exclude` block.
	ExcludeBlock
	// ErrorsBlock decodes the `errors` block.
	ErrorsBlock
	// TerraformExtraArgs decodes the `source` attribute and `extra_arguments` blocks of the `terraform` block.
	TerraformExtraArgs
	// OutputsBlock decodes the `outputs` block.
	OutputsBlock
	// moduleDirAttributes decodes the attributes that select the directory in which OpenTofu/Terraform runs.
	moduleDirAttributes
)

// terragruntIncludeMultiple is a struct that can be used to only decode the include block with labels.
type terragruntIncludeMultiple struct {
	Remain  hcl.Body       `hcl:",remain"`
	Include IncludeConfigs `hcl:"include,block"`
}

// terragruntFeatureFlags is a struct that can be used to store decoded feature flags.
type terragruntFeatureFlags struct {
	Remain       hcl.Body     `hcl:",remain"`
	FeatureFlags FeatureFlags `hcl:"feature,block"`
}

// TerraformSourceReferencesDependency reports whether the terraform block's `source` attribute references the
// `dependency` namespace anywhere. The module source is consumed during discovery, `--filter 'source='` matching,
// and queue construction, so a source that names a dependency
// can never be satisfied there and is rejected outright.
//
// Returns false for JSON configs, whose body is not hclsyntax.
func TerraformSourceReferencesDependency(file *hclparse.File) bool {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return false
	}

	for _, block := range body.Blocks {
		if block.Type != MetadataTerraform {
			continue
		}

		sourceAttr, ok := block.Body.Attributes["source"]
		if !ok {
			continue
		}

		if expressionReferencesDependency(sourceAttr.Expr) {
			return true
		}
	}

	return false
}

// ValidateExcludeReferences checks that no attribute of an `exclude` block in file references the `dependency`
// namespace.
//
// Returns [ExcludeReferencesDependencyError] when one does, and the diagnostics of an exclude block whose
// attributes fail to parse.
func ValidateExcludeReferences(file *hclparse.File) error {
	blocks, err := file.Blocks(MetadataExclude, true)
	if err != nil {
		return err
	}

	for _, block := range blocks {
		attrs, err := block.JustAttributes()
		if err != nil {
			return err
		}

		for _, attr := range attrs {
			if expressionReferencesDependency(attr.Expr) {
				return ExcludeReferencesDependencyError{ConfigPath: file.ConfigPath}
			}
		}
	}

	return nil
}

// ValidateVersionAttributeReferences reports a diagnostic through file for each of the top-level `terraform_binary`,
// `terraform_version_constraint`, and `terragrunt_version_constraint` attributes of file that references the
// `dependency` namespace, even in a branch that never evaluates. Terragrunt reads them to check versions before any
// dependency has run.
//
// Returns what [hclparse.File.HandleDiagnostics] returns for those diagnostics, or for the diagnostics of a body
// whose top-level content fails to parse.
func ValidateVersionAttributeReferences(file *hclparse.File) error {
	names := []string{
		MetadataTerraformBinary,
		MetadataTerraformVersionConstraint,
		MetadataTerragruntVersionConstraint,
	}

	schema := &hcl.BodySchema{Attributes: make([]hcl.AttributeSchema, 0, len(names))}
	for _, name := range names {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}

	content, _, diags := file.Body.PartialContent(schema)
	if err := file.HandleDiagnostics(diags); err != nil {
		return err
	}

	var refDiags hcl.Diagnostics

	for _, name := range names {
		attr, ok := content.Attributes[name]
		if !ok {
			continue
		}

		traversals := attr.Expr.Variables()

		reference := slices.IndexFunc(traversals, func(traversal hcl.Traversal) bool {
			return traversal.RootName() == MetadataDependency
		})
		if reference < 0 {
			continue
		}

		refDiags = refDiags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Dependency reference not allowed",
			Detail: fmt.Sprintf(
				"The %s attribute can't reference dependency outputs, because Terragrunt checks versions before "+
					"any dependency has run.",
				name,
			),
			Subject: traversals[reference].SourceRange().Ptr(),
			Context: attr.Range.Ptr(),
		})
	}

	return file.HandleDiagnostics(refDiags)
}

func expressionReferencesDependency(expr hcl.Expression) bool {
	for _, traversal := range expr.Variables() {
		if traversal.RootName() == MetadataDependency {
			return true
		}
	}

	return false
}

// TerragruntDependency is a struct that can be used to only decode the dependency blocks in the terragrunt config
type TerragruntDependency struct {
	Remain       hcl.Body     `hcl:",remain"`
	Dependencies Dependencies `hcl:"dependency,block"`
}

// DecodeBaseBlocks takes in a parsed HCL2 file and decodes the base blocks. Base blocks are blocks that should always
// be decoded even in partial decoding, because they provide bindings that are necessary for parsing any block in the
// file. Currently base blocks are:
// - locals
// - features
// - include
//
// The `include` variable the locals read holds what exposed gives for each exposed include block.
func DecodeBaseBlocks(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	s *Store,
	pctx *ParsingContext,
	file *hclparse.File,
	includeFromChild *IncludeConfig,
	included IncludedFeatureFlags,
	exposed ExposedInclude,
) (*DecodedBaseBlocks, error) {
	var errs []error

	evalParsingContext, err := CreateTerragruntEvalContext(
		ctx,
		l,
		v,
		s,
		pctx,
		file.ConfigPath,
		file.FunctionCalls(),
		exposed,
	)
	if err != nil {
		return nil, err
	}

	// Decode just the `include` and `import` blocks, and verify that it's allowed here
	terragruntIncludeList, err := decodeAsTerragruntInclude(
		file,
		evalParsingContext,
	)
	if err != nil {
		errs = append(errs, err)
	}

	trackInclude, err := getTrackInclude(pctx, terragruntIncludeList, includeFromChild)
	if err != nil {
		errs = append(errs, err)
	}

	registerSiblingAutoInclude(v, pctx, file.ConfigPath, trackInclude)

	// set feature flags
	tgFlags := terragruntFeatureFlags{}
	// load default feature flags
	if err := file.Decode(&tgFlags, evalParsingContext); err != nil {
		return nil, err
	}
	// validate flags to have default value, collect errors
	var flagErrs []error

	for _, flag := range tgFlags.FeatureFlags {
		if flag.Default == nil {
			flagErr := fmt.Errorf(
				"feature flag %s does not have a default value in %s",
				flag.Name,
				file.ConfigPath,
			)
			flagErrs = append(flagErrs, flagErr)
		}
	}

	errs = append(errs, flagErrs...)

	// Feature defaults from includes must be visible while decoding base blocks like
	// locals and exclude, but they must remain local to this parse instead of being
	// stored in shared command options.
	mergedFeatureFlags, err := mergeIncludedFeatureFlags(ctx, l, trackInclude, tgFlags.FeatureFlags, included)
	if err != nil {
		errs = append(errs, err)
	}

	flagsAsCtyVal, err := flagsAsCty(pctx, mergedFeatureFlags)
	if err != nil {
		errs = append(errs, err)
	}

	// Evaluate all the expressions in the locals block separately and generate the variables list to use in the
	// evaluation ctx.
	locals, err := EvaluateLocalsBlock(
		ctx,
		l,
		v,
		s,
		pctx.WithTrackInclude(trackInclude).WithFeatures(&flagsAsCtyVal),
		file,
		exposed,
	)
	if err != nil {
		errs = append(errs, err)
	}

	localsAsCtyVal, err := ConvertValuesMapToCtyVal(locals)
	if err != nil {
		return nil, err
	}

	return &DecodedBaseBlocks{
		TrackInclude:      trackInclude,
		Locals:            &localsAsCtyVal,
		FeatureFlags:      &flagsAsCtyVal,
		FeatureFlagBlocks: tgFlags.FeatureFlags,
	}, errors.Join(errs...)
}

// mergeIncludedFeatureFlags merges the feature defaults included gives for each config trackInclude lists into
// childFlags, last include first.
func mergeIncludedFeatureFlags(
	ctx context.Context,
	l log.Logger,
	trackInclude *TrackInclude,
	childFlags FeatureFlags,
	included IncludedFeatureFlags,
) (FeatureFlags, error) {
	if trackInclude == nil || len(trackInclude.CurrentList) == 0 {
		return childFlags, nil
	}

	baseConfig := &TerragruntConfig{FeatureFlags: childFlags}

	for _, includeConfig := range slices.Backward(trackInclude.CurrentList) {
		mergeStrategy, err := includeConfig.GetMergeStrategy()
		if err != nil {
			return childFlags, err
		}

		if mergeStrategy == NoMerge {
			continue
		}

		includedFlags, err := included(ctx, l, trackInclude, &includeConfig)
		if err != nil {
			return childFlags, err
		}

		mergedConfig, err := mergeFeatureFlagConfig(l, mergeStrategy, baseConfig, copyFeatureFlags(includedFlags))
		if err != nil {
			return childFlags, err
		}

		baseConfig = mergedConfig
	}

	return baseConfig.FeatureFlags, nil
}

// copyFeatureFlags copies each flag, so a merge that updates the copies in place leaves flags unchanged.
func copyFeatureFlags(flags FeatureFlags) FeatureFlags {
	copied := make(FeatureFlags, 0, len(flags))

	for _, flag := range flags {
		copied = append(copied, new(*flag))
	}

	return copied
}

// mergeFeatureFlagConfig applies an include merge strategy to feature defaults only.
func mergeFeatureFlagConfig(
	l log.Logger,
	mergeStrategy MergeStrategyType,
	baseConfig *TerragruntConfig,
	includeFlags FeatureFlags,
) (*TerragruntConfig, error) {
	includeOnlyConfig := &TerragruntConfig{FeatureFlags: includeFlags}

	switch mergeStrategy {
	case NoMerge:
		return baseConfig, nil
	case ShallowMerge:
		if err := includeOnlyConfig.Merge(l, baseConfig); err != nil {
			return nil, err
		}
	case DeepMerge:
		if err := includeOnlyConfig.DeepMerge(l, baseConfig); err != nil {
			return nil, err
		}
	case DeepMergeMapOnly:
		return nil, IncludeMergeStrategyNotSupportedError(mergeStrategy)
	default:
		return nil, fmt.Errorf(
			"you reached an impossible condition. "+
				"This is most likely a bug in terragrunt. "+
				"Please open an issue at github.com/gruntwork-io/terragrunt "+
				"with this error message. Code: UNKNOWN_MERGE_STRATEGY_%s",
			mergeStrategy,
		)
	}

	return includeOnlyConfig, nil
}

// flagsAsCty converts feature flag defaults and CLI overrides into the evaluation context value.
func flagsAsCty(ctx *ParsingContext, tgFlags FeatureFlags) (cty.Value, error) {
	// extract all flags in map by name
	flagByName := map[string]*FeatureFlag{}
	for _, flag := range tgFlags {
		flagByName[flag.Name] = flag
	}

	evaluatedFlags, err := cliFlagsToCty(ctx, flagByName)
	if err != nil {
		return cty.NilVal, err
	}

	var errs []error

	for _, flag := range tgFlags {
		if _, exists := evaluatedFlags[flag.Name]; !exists {
			if flag.Default == nil {
				errs = append(
					errs,
					fmt.Errorf(
						"feature flag %s does not have a default value in %s",
						flag.Name,
						ctx.TerragruntConfigPath,
					),
				)

				continue
			}

			contextFlag, err := flagToCtyValue(flag.Name, *flag.Default)
			if err != nil {
				return cty.NilVal, err
			}

			evaluatedFlags[flag.Name] = contextFlag
		}
	}

	flagsAsCtyVal, err := ConvertValuesMapToCtyVal(evaluatedFlags)
	if err != nil {
		return cty.NilVal, err
	}

	return flagsAsCtyVal, errors.Join(errs...)
}

// cliFlagsToCty converts CLI feature flags to Cty values. It returns a map of flag names
// to their corresponding Cty values and any error encountered during conversion.
func cliFlagsToCty(
	ctx *ParsingContext,
	flagByName map[string]*FeatureFlag,
) (map[string]cty.Value, error) {
	evaluatedFlags := make(map[string]cty.Value)

	for name, value := range ctx.FeatureFlags {
		var flag cty.Value

		var err error

		if existingFlag, ok := flagByName[name]; ok {
			flag, err = flagToTypedCtyValue(name, existingFlag.Default.Type(), value)
		} else {
			flag, err = flagToCtyValue(name, value)
		}

		if err != nil {
			return nil, err
		}

		evaluatedFlags[name] = flag
	}

	return evaluatedFlags, nil
}

// decodeAsTerragruntInclude decodes only the include blocks of a Terragrunt config, so its value can be used while
// decoding the rest of the config.
// For consistency, `include` in the call to `file.Decode` is always assumed to be nil. Either it really is nil (parsing
// the child config), or it shouldn't be used anyway (the parent config shouldn't have an include block).
func decodeAsTerragruntInclude(
	file *hclparse.File,
	evalParsingContext *hcl.EvalContext,
) (IncludeConfigs, error) {
	tgInc := terragruntIncludeMultiple{}
	if err := file.Decode(&tgInc, evalParsingContext); err != nil {
		return nil, err
	}

	return tgInc.Include, nil
}

// registerSiblingAutoInclude records the sibling terragrunt.autoinclude.hcl on trackInclude as a high-priority override when it is in scope and exists, so the merge consumers read one registered entry instead of recomputing the gate.
func registerSiblingAutoInclude(
	v *venv.Venv,
	pctx *ParsingContext,
	cfgPath string,
	trackInclude *TrackInclude,
) {
	if trackInclude == nil {
		return
	}

	autoIncludePath, ok := siblingAutoIncludePath(pctx, cfgPath)
	if !ok {
		return
	}

	if !vfs.Exists(v.FS, autoIncludePath) {
		return
	}

	trackInclude.AutoIncludeOverride = &IncludeConfig{Path: autoIncludePath}
}

// moduleDirDependencyError returns the error for a module directory attribute of file that references a dependency.
func moduleDirDependencyError(file *hclparse.File) error {
	const source = "source"

	attr, err := dependencyReferenceAttribute(file, file.Body, MetadataDownloadDir)
	if err != nil {
		return err
	}

	if attr != "" {
		return ModuleDirReferencesDependencyError{ConfigPath: file.ConfigPath, Attribute: attr}
	}

	blocks, err := file.Blocks(MetadataTerraform, true)
	if err != nil {
		return err
	}

	for _, block := range blocks {
		attr, err := dependencyReferenceAttribute(file, block.Body, source, "no_dot_terragrunt_cache")
		if err != nil {
			return err
		}

		if attr == source {
			return TerraformSourceReferencesDependencyError{ConfigPath: file.ConfigPath}
		}

		if attr != "" {
			return ModuleDirReferencesDependencyError{
				ConfigPath: file.ConfigPath,
				Attribute:  MetadataTerraform + "." + attr,
			}
		}
	}

	return nil
}

// dependencyReferenceAttribute returns the first of the named attributes of body that references a dependency, or "".
func dependencyReferenceAttribute(file *hclparse.File, body hcl.Body, names ...string) (string, error) {
	schema := &hcl.BodySchema{Attributes: make([]hcl.AttributeSchema, 0, len(names))}
	for _, name := range names {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}

	content, _, diags := body.PartialContent(schema)
	if err := file.HandleDiagnostics(diags); err != nil {
		return "", err
	}

	for _, name := range names {
		if attr, found := content.Attributes[name]; found && expressionReferencesDependency(attr.Expr) {
			return name, nil
		}
	}

	return "", nil
}

// Custom error types
