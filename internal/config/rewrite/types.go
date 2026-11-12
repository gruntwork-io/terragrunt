package config

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// UnitConfig is a parsed config file with the autoinclude and included files it merges.
// [UnitConfig.ToV1] assembles them into the config [pkgconfig.ParseConfigFile] returns.
//
// The tagged fields decode in the parse, which fetches no dependency outputs, so an attribute among them cannot
// read `dependency`. The exceptions are the top-level attributes [pkgconfig.ParseConfig] decodes after it resolves
// the outputs: one of those that reads `dependency` is a run part. The Remain of each block, and each [hcl.Attribute] field, is a queue part or a run part. The
// parse decodes the queue parts, which [QueueBodies] names, into Queue, with no `dependency` variable in scope, or an
// unknown one for the engine block.
// ToV1 decodes the run parts, which [RunBodies] names, with the outputs resolved. An exclude block that reads
// `dependency` is a run part. A nil block field means the file has no such block.
//
// A UnitConfig is shared between the callers of a memoized parse. It holds none of a caller's settings or sinks, and
// nothing changes it once its parse returns.
type UnitConfig struct {
	// Catalog is the catalog block.
	Catalog *CatalogHCL `hcl:"catalog,block"`
	// Engine is the engine block.
	Engine *EngineHCL `hcl:"engine,block"`
	// Terraform is the terraform block.
	Terraform *TerraformHCL `hcl:"terraform,block"`
	// TerraformBinary is the terraform_binary attribute.
	TerraformBinary *string `hcl:"terraform_binary,attr"`
	// TerraformVersionConstraint is the terraform_version_constraint attribute.
	TerraformVersionConstraint *string `hcl:"terraform_version_constraint,attr"`
	// TerragruntVersionConstraint is the terragrunt_version_constraint attribute.
	TerragruntVersionConstraint *string `hcl:"terragrunt_version_constraint,attr"`
	// Inputs is the inputs attribute.
	Inputs *hcl.Attribute `hcl:"inputs,attr"`
	// RemoteState is the remote_state block.
	RemoteState *RemoteStateHCL `hcl:"remote_state,block"`
	// RemoteStateAttr is the remote_state attribute.
	RemoteStateAttr *hcl.Attribute `hcl:"remote_state,optional"`
	// Dependencies is the dependencies block.
	Dependencies *ModuleDependenciesHCL `hcl:"dependencies,block"`
	// DownloadDir is the download_dir attribute.
	DownloadDir *string `hcl:"download_dir,attr"`
	// PreventDestroy is the prevent_destroy attribute.
	PreventDestroy *bool `hcl:"prevent_destroy,attr"`
	// IamRole is the iam_role attribute.
	IamRole *string `hcl:"iam_role,attr"`
	// IamAssumeRoleDuration is the iam_assume_role_duration attribute.
	IamAssumeRoleDuration *int64 `hcl:"iam_assume_role_duration,attr"`
	// IamAssumeRoleSessionName is the iam_assume_role_session_name attribute.
	IamAssumeRoleSessionName *string `hcl:"iam_assume_role_session_name,attr"`
	// IamWebIdentityToken is the iam_web_identity_token attribute.
	IamWebIdentityToken *string `hcl:"iam_web_identity_token,attr"`
	// DependencyBlocks is the dependency blocks, in source order.
	DependencyBlocks []DependencyHCL `hcl:"dependency,block"`
	// FeatureFlags is the feature blocks, in source order.
	FeatureFlags []FeatureFlagHCL `hcl:"feature,block"`
	// Exclude is the exclude block.
	Exclude *ExcludeHCL `hcl:"exclude,block"`
	// Errors is the errors block.
	Errors *ErrorsHCL `hcl:"errors,block"`
	// Outputs is the outputs block.
	Outputs *OutputsHCL `hcl:"outputs,block"`
	// GenerateAttrs is the generate attribute.
	GenerateAttrs *hcl.Attribute `hcl:"generate,optional"`
	// GenerateBlocks is the generate blocks, in source order.
	GenerateBlocks []GenerateHCL `hcl:"generate,block"`
	// VariablesBlocks is the variables blocks, in source order.
	VariablesBlocks []VariablesHCL `hcl:"variables,block"`
	// LocalsBlock is the locals block.
	LocalsBlock *LocalsHCL `hcl:"locals,block"`
	// IncludeBlocks is the include blocks, in source order.
	IncludeBlocks []IncludeHCL `hcl:"include,block"`
	// Queue is the queue parts decoded.
	Queue QueueBodies
	// DependencyConfigs is the decoded dependency blocks, merged with those of includes and the autoinclude. It is
	// empty for an included file.
	DependencyConfigs pkgconfig.Dependencies
	// Includes is the evaluated include blocks, nil when the base blocks failed to decode.
	Includes *Includes
	// Locals is the evaluated locals block.
	Locals *cty.Value

	// file is the parsed file, with no parser.
	file *pkghclparse.File
	// evalVars is the variables the blocks decoded with, nil when the eval context failed to build.
	evalVars map[string]cty.Value
	// evalErr is the error of building the eval context, which prepErrs also holds.
	evalErr error
	// deps is the dependency blocks of a file that resolves its own dependencies, nil when they failed to decode or
	// an including file resolves them.
	deps *dependencySet
	// dependencyReads is the files decoding the dependency blocks read, which only a run reports.
	dependencyReads []string
	// decodeErr is the error of the file update before the decode, which leaves the blocks empty, or of the
	// parse's decode diagnostics.
	decodeErr error
	// dependencyAttrs is the top-level attributes that read `dependency` among those ToV1 decodes with the outputs,
	// by name. The tagged field of each stays nil.
	dependencyAttrs map[string]*hcl.Attribute
	// queueUnknownDiags is the diagnostics of decoding Queue.Engine and the queue attributes in dependencyAttrs with
	// `dependency` unknown, which only a queue assembly reports. ToV1 decodes them again with the outputs and
	// reports its own.
	queueUnknownDiags hcl.Diagnostics
	// prepErrs is the non-fatal errors of the stages before the decode.
	prepErrs []error
	// kept is the files the parse merged.
	kept keptFiles
	// scope is the file's state after its base blocks decoded.
	scope fileScope
}

// keptFiles is the files a parse reached while merging a config.
type keptFiles struct {
	// autoInclude is the merged autoinclude, nil when the parse did not reach one.
	autoInclude *includedFile
	// included is the file each include block pulls in, nil where the parse did not reach it.
	included []*includedFile
}

// newKeptFiles returns keptFiles with room for includes, which is nil for a file with no include blocks.
func newKeptFiles(includes *Includes) *keptFiles {
	if includes == nil {
		return &keptFiles{}
	}

	return &keptFiles{included: make([]*includedFile, len(includes.List))}
}

// Includes is the include blocks of a file and, for an included file, the include block it is parsed through.
type Includes struct {
	// IncludedVia is the include block that pulls the file in, nil for a component's own config.
	IncludedVia *pkgconfig.IncludeConfig
	// AutoIncludePath is the sibling terragrunt.autoinclude.hcl, empty when there is none. List leaves it out.
	AutoIncludePath string
	// List is the include blocks, in source order.
	List pkgconfig.IncludeConfigs
}

// newIncludes converts the includes pkg/config decoded, or returns nil for none.
func newIncludes(trackInclude *pkgconfig.TrackInclude) *Includes {
	if trackInclude == nil {
		return nil
	}

	includes := &Includes{
		List:        trackInclude.CurrentList,
		IncludedVia: trackInclude.Original,
	}

	if trackInclude.AutoIncludeOverride != nil {
		includes.AutoIncludePath = trackInclude.AutoIncludeOverride.Path
	}

	return includes
}

// ByName returns the include blocks of List by label. A repeated label keeps the last block.
func (i *Includes) ByName() pkgconfig.IncludeConfigsMap {
	byName := make(pkgconfig.IncludeConfigsMap, len(i.List))

	for _, include := range i.List {
		byName[include.Name] = include
	}

	return byName
}

// includedFile is the parse of an included file or autoinclude.
type includedFile struct {
	// cfg is the parsed file, nil when its parse failed before the decode.
	cfg *UnitConfig
	// err is the failure that left cfg nil.
	err error
}
