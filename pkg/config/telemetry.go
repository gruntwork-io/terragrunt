// Package config provides telemetry support for configuration parsing operations.
package config

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Telemetry operation names for config parsing operations.
const (
	TelemetryOpParseConfigFile       = "parse_config_file"
	TelemetryOpParseBaseBlocks       = "parse_base_blocks"
	TelemetryOpParseBaseBlocksResult = "parse_base_blocks_result"
	TelemetryOpParseDependencies     = "parse_dependencies"
	TelemetryOpParseDependency       = "parse_dependency"
	TelemetryOpParseConfigDecode     = "parse_config_decode"
	TelemetryOpParseIncludeMerge     = "parse_include_merge"
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
	AttrHasIncludes      = "has_includes"
	AttrIncludeCount     = "include_count"
	AttrIncludePaths     = "include_paths"
	AttrDependencyCount  = "dependency_count"
	AttrDependencyNames  = "dependency_names"
	AttrDependencyName   = "dependency_name"
	AttrDependencyPath   = "dependency_path"
	AttrLocalsCount      = "locals_count"
	AttrLocalsNames      = "locals_names"
	AttrFeatureFlagCount = "feature_flag_count"
	AttrFeatureFlagNames = "feature_flag_names"
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

// TraceParseBaseBlocks wraps base blocks parsing with telemetry.
func TraceParseBaseBlocks(
	ctx context.Context,
	l log.Logger,
	configPath string,
	fn func(ctx context.Context) (*DecodedBaseBlocks, error),
) (*DecodedBaseBlocks, error) {
	var (
		result    *DecodedBaseBlocks
		resultErr error
	)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseBaseBlocks, map[string]any{
		AttrConfigPath: configPath,
	}, func(childCtx context.Context) error {
		result, resultErr = fn(childCtx)
		return resultErr
	})
	if err != nil {
		l.Warnf("Telemetry error during base blocks parsing: %v", err)
	}

	return result, resultErr
}

// TraceParseBaseBlocksResult adds result attributes to the current span from context.
func TraceParseBaseBlocksResult(
	ctx context.Context,
	configPath string,
	baseBlocks *DecodedBaseBlocks,
) {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String(AttrConfigPath, configPath),
	}

	if baseBlocks != nil {
		// Include information
		if baseBlocks.TrackInclude != nil && len(baseBlocks.TrackInclude.CurrentList) > 0 {
			attrs = append(attrs,
				attribute.Bool(AttrHasIncludes, true),
				attribute.Int(AttrIncludeCount, len(baseBlocks.TrackInclude.CurrentList)),
				attribute.String(AttrIncludePaths, formatIncludePaths(baseBlocks.TrackInclude.CurrentList)),
			)
		} else {
			attrs = append(attrs,
				attribute.Bool(AttrHasIncludes, false),
				attribute.Int(AttrIncludeCount, 0),
			)
		}

		// Locals information
		if baseBlocks.Locals != nil && !baseBlocks.Locals.IsNull() {
			localsMap := baseBlocks.Locals.AsValueMap()
			attrs = append(attrs,
				attribute.Int(AttrLocalsCount, len(localsMap)),
				attribute.String(AttrLocalsNames, formatMapKeys(localsMap)),
			)
		} else {
			attrs = append(attrs, attribute.Int(AttrLocalsCount, 0))
		}

		// Feature flags information
		if baseBlocks.FeatureFlags != nil && !baseBlocks.FeatureFlags.IsNull() {
			flagsMap := baseBlocks.FeatureFlags.AsValueMap()
			attrs = append(attrs,
				attribute.Int(AttrFeatureFlagCount, len(flagsMap)),
				attribute.String(AttrFeatureFlagNames, formatMapKeys(flagsMap)),
			)
		} else {
			attrs = append(attrs, attribute.Int(AttrFeatureFlagCount, 0))
		}
	}

	span.SetAttributes(attrs...)
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

// TraceParseDependency wraps individual dependency output resolution with telemetry.
func TraceParseDependency(
	ctx context.Context,
	dependencyName string,
	dependencyPath string,
	fn func(ctx context.Context) error,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseDependency, map[string]any{
		AttrDependencyName: dependencyName,
		AttrDependencyPath: dependencyPath,
	}, fn)
}

// TraceParseConfigDecode wraps config decoding with telemetry.
func TraceParseConfigDecode(
	ctx context.Context,
	configPath string,
	fn func(ctx context.Context) error,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseConfigDecode, map[string]any{
		AttrConfigPath: configPath,
	}, fn)
}

// TraceParseIncludeMerge wraps include merging with telemetry.
func TraceParseIncludeMerge(
	ctx context.Context,
	configPath string,
	includeCount int,
	includePaths []string,
	fn func(ctx context.Context) error,
) error {
	attrs := map[string]any{
		AttrConfigPath:   configPath,
		AttrIncludeCount: includeCount,
	}

	if len(includePaths) > 0 {
		attrs[AttrIncludePaths] = strings.Join(includePaths, ",")
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, TelemetryOpParseIncludeMerge, attrs, fn)
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

// formatIncludePaths extracts and formats include paths from a list of IncludeConfigs.
func formatIncludePaths(includes IncludeConfigs) string {
	paths := make([]string, 0, len(includes))
	for _, inc := range includes {
		if inc.Path != "" {
			paths = append(paths, inc.Path)
		}
	}

	return strings.Join(paths, ",")
}

// formatMapKeys extracts keys from a map and returns them as a comma-separated string.
func formatMapKeys[V any](m map[string]V) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return strings.Join(keys, ",")
}
