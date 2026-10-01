package config

import (
	"context"
	"fmt"
	"hash/maphash"
	"io"
	"maps"
	"slices"
	"sync/atomic"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// readTerragruntConfigResult is a read_terragrunt_config result shared by every config that reads the same target
// with the same [ReadTerragruntConfigKey].
type readTerragruntConfigResult struct {
	value     cty.Value
	filesRead []string
}

// ReadTerragruntConfigKey identifies a read_terragrunt_config target and the inputs its parse takes from the reading
// config's context. Reads with equal keys in one command share one result.
//
// [ParseTerragruntConfig] resets the fields the target has to compute for itself, and the functions that read the
// reading config's path stop the result being shared, so neither appears here.
type ReadTerragruntConfigKey struct {
	TerragruntStackConfigPath string
	ConfigPath                string
	TerraformCommand          string
	OriginalTerraformCommand  string
	Source                    string
	DownloadDir               string
	OriginalIAMRoleOptions    iam.RoleOptions
	IAMRoleOptions            iam.RoleOptions
	HaltOnErrorOnlyInBlocks   []string
	TerraformCliArgs          []string
	FeatureFlags              []string
	DecodeList                []PartialDecodeSectionType
	ConfigModTime             int64
	EnvHash                   uint64
	MaxFoldersToCheck         int
	Diagnostics               DiagnosticsOutput
	DiscardOutput             bool
	TrackFilesRead            bool
	SkipOutput                bool
	TFPathExplicitlySet       bool
	RewriteBareInclude        bool
	IgnoreDiagnostics         bool
	SkipDefaults              bool
	StubWorkingDirFunc        bool
	CatalogOnly               bool
	SkipAutoIncludeMerge      bool
}

// shareabilityKey is the context key for the flags of the read_terragrunt_config parses in flight on a call path.
type shareabilityKey struct{}

// perCallFunctionNames lists the OpenTofu functions that return a new value on every call.
var perCallFunctionNames = []string{"bcrypt", "timestamp", "uuid"}

// envHashSeed seeds the environment hash in [ReadTerragruntConfigKey]. The hash only has to agree within one process.
var envHashSeed = maphash.MakeSeed()

// NewReadTerragruntConfigKey builds the cache key for reading the target at pctx.TerragruntConfigPath, last modified
// at modTime.
func NewReadTerragruntConfigKey(v *venv.Venv, pctx *ParsingContext, modTime int64) ReadTerragruntConfigKey {
	var cliArgs []string
	if pctx.TerraformCliArgs != nil {
		cliArgs = pctx.TerraformCliArgs.Slice()
	}

	featureFlags := make([]string, 0, len(pctx.FeatureFlags))
	for _, name := range slices.Sorted(maps.Keys(pctx.FeatureFlags)) {
		featureFlags = append(featureFlags, name+"="+pctx.FeatureFlags[name])
	}

	return ReadTerragruntConfigKey{
		IAMRoleOptions:            pctx.IAMRoleOptions,
		OriginalIAMRoleOptions:    pctx.OriginalIAMRoleOptions,
		ConfigPath:                pctx.TerragruntConfigPath,
		ConfigModTime:             modTime,
		TerraformCommand:          pctx.TerraformCommand,
		OriginalTerraformCommand:  pctx.OriginalTerraformCommand,
		Source:                    pctx.Source,
		DownloadDir:               pctx.DownloadDir,
		TerragruntStackConfigPath: pctx.TerragruntStackConfigPath,
		TerraformCliArgs:          cliArgs,
		FeatureFlags:              featureFlags,
		DecodeList:                pctx.PartialParseDecodeList,
		HaltOnErrorOnlyInBlocks:   pctx.Parser.HaltOnErrorOnlyInBlocks,
		EnvHash:                   hashEnv(v.Env),
		MaxFoldersToCheck:         pctx.MaxFoldersToCheck,
		Diagnostics:               pctx.Parser.Diagnostics,
		DiscardOutput:             v.Writers.Writer == io.Discard,
		TrackFilesRead:            pctx.FilesRead.Tracking(),
		SkipOutput:                pctx.SkipOutput,
		TFPathExplicitlySet:       pctx.TFPathExplicitlySet,
		RewriteBareInclude:        pctx.Parser.RewriteBareInclude,
		IgnoreDiagnostics:         pctx.Parser.IgnoreDiagnostics,
		SkipDefaults:              pctx.Parser.SkipDefaults,
		StubWorkingDirFunc:        pctx.stubWorkingDirFunc,
		CatalogOnly:               pctx.catalogOnly,
		SkipAutoIncludeMerge:      pctx.SkipAutoIncludeMerge,
	}
}

// readTerragruntConfigCached parses the read_terragrunt_config target at pctx.TerragruntConfigPath and converts it to
// a cty value, reusing the value an earlier read produced with the same key in this command.
//
// A parse whose value another read with the same key could not reuse marks itself and every parse in flight around it
// as not shareable, and its value is not shared. That covers reading the reading config's path, a target with
// dependency blocks, run_cmd with --terragrunt-no-cache, and the functions in perCallFunctionNames.
func readTerragruntConfigCached(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (cty.Value, error) {
	if pctx.Parser.DiagnosticsHandler != nil {
		return parseTerragruntConfigAsCty(ctx, l, v, pctx)
	}

	fileInfo, err := v.FS.Stat(pctx.TerragruntConfigPath)
	if err != nil {
		return cty.NilVal, err
	}

	key := fmt.Sprintf("%+v", NewReadTerragruntConfigKey(v, pctx, fileInfo.ModTime().UnixMicro()))
	results := cache.ContextCache[*readTerragruntConfigResult](ctx, ReadTerragruntConfigCacheContextKey)

	if result, found := results.Get(ctx, key); found {
		for _, path := range result.filesRead {
			pctx.FilesRead.Add(path)
		}

		return result.value, nil
	}

	parseCtx, notShareable := withShareabilityFlag(ctx)

	parsePctx := pctx
	if pctx.FilesRead.Tracking() {
		parsePctx = pctx.Clone()
		parsePctx.FilesRead = NewFilesRead()
	}

	value, err := parseTerragruntConfigAsCty(parseCtx, l, v, parsePctx)

	filesRead := parsePctx.FilesRead.Paths()
	for _, path := range filesRead {
		pctx.FilesRead.Add(path)
	}

	if err != nil {
		return cty.NilVal, err
	}

	if !notShareable.Load() {
		results.Put(ctx, key, &readTerragruntConfigResult{value: value, filesRead: filesRead})
	}

	return value, nil
}

// parseTerragruntConfigAsCty parses the config at pctx.TerragruntConfigPath with its dependency outputs rendered,
// and converts it to a cty value.
func parseTerragruntConfigAsCty(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (cty.Value, error) {
	config, err := ParseConfigFile(ctx, l, v, pctx, pctx.TerragruntConfigPath, nil)
	if err != nil {
		return cty.NilVal, err
	}

	if len(config.TerragruntDependencies) > 0 {
		markNotShareable(ctx)
	}

	// We have to set the rendered outputs here because ParseConfigFile will not do so on the TerragruntConfig. The
	// outputs are stored in a special map that is used only for rendering and thus is not available when we try to
	// serialize the config for consumption.
	// NOTE: this will not call terragrunt output, since all the values are cached from the ParseConfigFile call
	// NOTE: we don't use range here because range will copy the slice, thereby undoing the set attribute.
	for i := range len(config.TerragruntDependencies) {
		err := config.TerragruntDependencies[i].setRenderedOutputs(ctx, l, v, pctx)
		if err != nil {
			return cty.NilVal, err
		}
	}

	return TerragruntConfigAsCty(config)
}

// hashEnv hashes env independently of its iteration order.
func hashEnv(env map[string]string) uint64 {
	var sum uint64

	for name, value := range env {
		sum += maphash.Comparable(envHashSeed, [2]string{name, value})
	}

	return sum
}

// withShareabilityFlag returns a context that tracks one more read_terragrunt_config parse, and the flag
// markNotShareable sets for it.
func withShareabilityFlag(ctx context.Context) (context.Context, *atomic.Bool) {
	flag := new(atomic.Bool)
	outer, _ := ctx.Value(shareabilityKey{}).([]*atomic.Bool)

	return context.WithValue(ctx, shareabilityKey{}, append(slices.Clone(outer), flag)), flag
}

// markNotShareable records that the read_terragrunt_config parses in flight on ctx produce a value another read with
// the same key could not reuse. It does nothing outside such a parse.
func markNotShareable(ctx context.Context) {
	flags, _ := ctx.Value(shareabilityKey{}).([]*atomic.Bool)
	for _, flag := range flags {
		flag.Store(true)
	}
}

// markingNotShareable wraps fn so every call marks the read_terragrunt_config parses in flight on ctx as not
// shareable before calling fn.
func markingNotShareable(ctx context.Context, fn function.Function) function.Function {
	return function.New(&function.Spec{
		Description: fn.Description(),
		Params:      fn.Params(),
		VarParam:    fn.VarParam(),
		Type:        fn.ReturnTypeForValues,
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			markNotShareable(ctx)

			return fn.Call(args)
		},
	})
}
