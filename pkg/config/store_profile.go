package config

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"maps"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// runDigest returns the digest of what a parse of run through v reads beyond the file, and false when run carries
// state the digest does not cover, which leaves the parse unmemoized.
//
// The digest covers the config, original config, working directory, and root working directory; the download
// directory, source, and source map; the tofu command, original command, and CLI args; the IAM role options and their
// original values; the feature flag overrides; the auth provider command; the parser settings; the
// read_terragrunt_config chain; SkipOutput; SkipOutputsResolution; the parse depth; SkipAutoIncludeMerge; whether
// the parse decodes only the catalog block, and the directory of a generated catalog child; whether get_working_dir
// is stubbed; whether reads are tracked; whether the parse is partial; the environment of v; and whether v discards
// output.
//
// It leaves out the experiments, the strict controls, the maximum number of parent folders to check, the engine,
// the tofu binary and implementation, the provider cache and CAS options, and the telemetry and logging options.
//
// The parse runs unmemoized for a supplied `dependency`, values, feature, or locals value, a supplied include
// list, and a diagnostics handler.
func runDigest(v *venv.Venv, run *ParsingContext) (string, bool) {
	if run.DecodedDependencies != nil || run.Values != nil || run.Features != nil || run.Locals != nil {
		return "", false
	}

	if run.TrackInclude != nil || run.Parser.DiagnosticsHandler != nil {
		return "", false
	}

	d := newDigest()

	d.str(run.TerragruntConfigPath)
	d.str(run.OriginalTerragruntConfigPath)
	d.str(run.WorkingDir)
	d.str(run.RootWorkingDir)
	d.str(run.DownloadDir)
	d.str(run.Source)
	d.strMap(run.SourceMap)
	d.str(run.TerraformCommand)
	d.str(run.OriginalTofuCommand)

	if run.TerraformCliArgs != nil {
		d.strs(run.TerraformCliArgs.Slice())
	}

	d.roleOptions(run.IAMRoleOptions)
	d.roleOptions(run.OriginalIAMRoleOptions)
	d.strMap(run.FeatureFlags)
	d.str(run.AuthProviderCmd)
	d.parserSettings(run.Parser)
	d.strs(run.ReadConfigChain())
	d.bool(run.SkipOutput)
	d.bool(run.SkipOutputsResolution)
	d.int(run.ParseDepth)
	d.bool(run.SkipAutoIncludeMerge)
	d.bool(run.catalogOnly)
	d.str(run.catalogStubDir)
	d.bool(run.stubWorkingDirFunc)
	d.bool(run.FilesRead.Tracking())
	d.bool(len(run.PartialParseDecodeList) > 0)
	d.uint64(HashEnv(v.Env))
	d.bool(v.Writers.Writer == io.Discard)

	return d.sum(), true
}

// extend returns the digest of key followed by parts.
func extend(key string, parts ...string) string {
	d := newDigest()
	d.str(key)
	d.strs(parts)

	return d.sum()
}

const decimal = 10

// digest accumulates typed parts into one hash, each followed by a separator.
type digest struct {
	buf []byte
}

func newDigest() *digest {
	return &digest{}
}

func (d *digest) str(s string) {
	d.buf = append(d.buf, s...)
	d.buf = append(d.buf, 0)
}

func (d *digest) strs(ss []string) {
	d.int(len(ss))

	for _, s := range ss {
		d.str(s)
	}
}

func (d *digest) strMap(m map[string]string) {
	d.int(len(m))

	keys := slices.AppendSeq(make([]string, 0, len(m)), maps.Keys(m))
	slices.Sort(keys)

	for _, k := range keys {
		d.str(k)
		d.str(m[k])
	}
}

func (d *digest) bool(b bool) {
	d.buf = strconv.AppendBool(d.buf, b)
	d.buf = append(d.buf, 0)
}

func (d *digest) int(n int) {
	d.int64(int64(n))
}

func (d *digest) int64(n int64) {
	d.buf = strconv.AppendInt(d.buf, n, decimal)
	d.buf = append(d.buf, 0)
}

func (d *digest) uint64(n uint64) {
	d.buf = strconv.AppendUint(d.buf, n, decimal)
	d.buf = append(d.buf, 0)
}

func (d *digest) roleOptions(o iam.RoleOptions) {
	d.str(o.RoleARN)
	d.str(o.WebIdentityToken)
	d.str(o.AssumeRoleSessionName)
	d.int64(o.AssumeRoleDuration)
}

func (d *digest) parserSettings(s ParserSettings) {
	d.int(int(s.Diagnostics))
	d.bool(s.RewriteBareInclude)
	d.bool(s.IgnoreDiagnostics)
	d.bool(s.SkipDefaults)
	d.strs(s.HaltOnErrorOnlyInBlocks)
}

func (d *digest) sum() string {
	sum := sha256.Sum256(d.buf)

	return hex.EncodeToString(sum[:])
}
