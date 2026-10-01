package config

import (
	"context"
	"fmt"
	"hash/maphash"
	"io"
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// readTerragruntConfigResult is a read_terragrunt_config result shared by every config that reads the same target
// with the same [ReadTerragruntConfigKey], as long as none of the files the parse read has changed since.
type readTerragruntConfigResult struct {
	value cty.Value
	// files holds every file the parse read, including the target and the files its includes and nested reads
	// read, with what a stat of each said right after the parse.
	files map[string]fileStamp
}

// fileStamp is what a stat of a file said about it. Two stamps that differ mean the file changed in between.
type fileStamp struct {
	modTime time.Time
	size    int64
	exists  bool
}

// stampFile stats path. A file that is missing or unreadable gets the zero stamp.
func stampFile(fsys vfs.FS, path string) fileStamp {
	info, err := fsys.Stat(path)
	if err != nil {
		return fileStamp{}
	}

	return fileStamp{modTime: info.ModTime(), size: info.Size(), exists: true}
}

// current reports whether every file the parse read still has the stamp it had right after the parse.
func (r *readTerragruntConfigResult) current(fsys vfs.FS) bool {
	for path, stamp := range r.files {
		now := stampFile(fsys, path)
		if now.exists != stamp.exists || now.size != stamp.size || !now.modTime.Equal(stamp.modTime) {
			return false
		}
	}

	return true
}

// filesRead returns the paths of the files the parse read, in lexical order.
func (r *readTerragruntConfigResult) filesRead() []string {
	return slices.Sorted(maps.Keys(r.files))
}

// ReadTerragruntConfigKey identifies a read_terragrunt_config target and the inputs its parse takes from the reading
// config's context. Reads with equal keys in one command share one result.
//
// [ParseTerragruntConfig] resets the fields the target has to compute for itself, and the functions that read the
// reading config's path stop the result being shared, so neither appears here. The files the parse reads are not
// inputs either: the result records their stamps, and a read checks them before reusing it.
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
	EnvHash                   uint64
	MaxFoldersToCheck         int
	Diagnostics               DiagnosticsOutput
	DiscardOutput             bool
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

// NewReadTerragruntConfigKey builds the cache key for reading the target at pctx.TerragruntConfigPath.
func NewReadTerragruntConfigKey(v *venv.Venv, pctx *ParsingContext) ReadTerragruntConfigKey {
	// TerragruntOptions allows a nil TerraformCliArgs (see InsertTerraformCliArgs), configbridge passes it through,
	// and get_terraform_cli_args returns nothing for it.
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
		SkipOutput:                pctx.SkipOutput,
		TFPathExplicitlySet:       pctx.TFPathExplicitlySet,
		RewriteBareInclude:        pctx.Parser.RewriteBareInclude,
		IgnoreDiagnostics:         pctx.Parser.IgnoreDiagnostics,
		SkipDefaults:              pctx.Parser.SkipDefaults,
		StubWorkingDirFunc:        pctx.stubWorkingDirFunc,
		CatalogOnly:               pctx.catalogOnly,
		SkipAutoIncludeMerge:      pctx.skipAutoIncludeMerge,
	}
}

// readTerragruntConfigCached parses the read_terragrunt_config target at pctx.TerragruntConfigPath and converts it to
// a cty value, reusing the value an earlier read produced with the same key in this command while every file that
// parse read is unchanged.
//
// A parse whose value another read with the same key could not reuse marks itself and every parse in flight around it
// as not shareable, and its value is not shared. That covers reading the reading config's path, a target with
// dependency blocks, run_cmd with --terragrunt-no-cache, and the functions in perCallFunctionNames.
//
// The parse records the files it reads whether or not pctx does, so the result knows which files to check. A file
// rewritten within the resolution of the filesystem's timestamps, to the same size, is not seen as changed.
func readTerragruntConfigCached(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
) (cty.Value, error) {
	if pctx.Parser.DiagnosticsHandler != nil {
		return parseTerragruntConfigAsCty(ctx, l, v, pctx)
	}

	key := fmt.Sprintf("%#v", NewReadTerragruntConfigKey(v, pctx))
	results := cache.ContextCache[*readTerragruntConfigResult](ctx, ReadTerragruntConfigCacheContextKey)

	if result, found := results.Get(ctx, key); found && result.current(v.FS) {
		for _, path := range result.filesRead() {
			pctx.FilesRead.Add(path)
		}

		return result.value, nil
	}

	parseCtx, notShareable := withShareabilityFlag(ctx)

	parsePctx := pctx.Clone()
	parsePctx.FilesRead = NewFilesRead()

	value, err := parseTerragruntConfigAsCty(parseCtx, l, v, parsePctx)

	files := map[string]fileStamp{pctx.TerragruntConfigPath: stampFile(v.FS, pctx.TerragruntConfigPath)}

	for _, path := range parsePctx.FilesRead.Paths() {
		pctx.FilesRead.Add(path)

		files[path] = stampFile(v.FS, path)
	}

	if err != nil {
		return cty.NilVal, err
	}

	if !notShareable.Load() {
		results.Put(ctx, key, &readTerragruntConfigResult{value: value, files: files})
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
