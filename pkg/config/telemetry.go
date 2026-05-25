// Package config provides telemetry support for configuration parsing operations.
package config

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
)

// Telemetry operation names for config parsing operations.
const (
	TelemetryOpParseConfigFile   = "parse_config_file"
	TelemetryOpParseDependencies = "parse_dependencies"
)

// Telemetry attribute keys for config parsing operations.
const (
	AttrConfigPath       = "config_path"
	AttrWorkingDir       = "working_dir"
	AttrIsPartial        = "is_partial"
	AttrDecodeList       = "decode_list"
	AttrCacheHit         = "cache_hit"
	AttrIncludeFromChild = "include_from_child"
	AttrIncludeChildPath = "include_child_path"
	AttrDependencyCount  = "dependency_count"
	AttrDependencyNames  = "dependency_names"
	AttrSkipOutputs      = "skip_outputs_resolution"
)

// TraceParseConfigFile wraps a config file parsing operation with telemetry.
func TraceParseConfigFile(
	ctx context.Context,
	configPath string,
	workingDir string,
	isPartial bool,
	decodeList []PartialDecodeSectionType,
	includeFromChild *IncludeConfig,
	cacheHit bool,
	fn func(ctx context.Context) error,
) error {
	attrs := map[string]any{
		AttrConfigPath:       configPath,
		AttrWorkingDir:       workingDir,
		AttrIsPartial:        isPartial,
		AttrCacheHit:         cacheHit,
		AttrIncludeFromChild: includeFromChild != nil,
	}

	if len(decodeList) > 0 {
		attrs[AttrDecodeList] = formatDecodeList(decodeList)
	}

	if includeFromChild != nil {
		attrs[AttrIncludeChildPath] = includeFromChild.Path
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseConfigFile, attrs, fn)
}

// TraceParseDependencies wraps dependency parsing with telemetry.
func TraceParseDependencies(
	ctx context.Context,
	configPath string,
	skipOutputsResolution bool,
	dependencyCount int,
	dependencyNames []string,
	fn func(ctx context.Context) error,
) error {
	attrs := map[string]any{
		AttrConfigPath:      configPath,
		AttrSkipOutputs:     skipOutputsResolution,
		AttrDependencyCount: dependencyCount,
	}

	if len(dependencyNames) > 0 {
		attrs[AttrDependencyNames] = strings.Join(dependencyNames, ",")
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseDependencies, attrs, fn)
}

// formatDecodeList converts a slice of PartialDecodeSectionType to a comma-separated string.
func formatDecodeList(decodeList []PartialDecodeSectionType) string {
	names := make([]string, 0, len(decodeList))
	for _, section := range decodeList {
		names = append(names, partialDecodeSectionName(section))
	}

	return strings.Join(names, ",")
}

// partialDecodeSectionName returns a human-readable name for a PartialDecodeSectionType.
func partialDecodeSectionName(section PartialDecodeSectionType) string {
	switch section {
	case DependenciesBlock:
		return "dependencies"
	case DependencyBlock:
		return "dependency"
	case TerraformBlock:
		return "terraform"
	case TerraformSource:
		return "terraform_source"
	case TerragruntFlags:
		return "terragrunt_flags"
	case TerragruntVersionConstraints:
		return "version_constraints"
	case RemoteStateBlock:
		return "remote_state"
	case FeatureFlagsBlock:
		return "feature_flags"
	case EngineBlock:
		return "engine"
	case ExcludeBlock:
		return "exclude"
	case ErrorsBlock:
		return "errors"
	default:
		return "unknown"
	}
}
