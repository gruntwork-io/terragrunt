package config

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"maps"
	"slices"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
)

// runDigest returns the digest of the settings of run that a parse reads beyond the file, and false when run carries
// state the digest does not cover, which leaves the parse unmemoized.
//
// The digest covers the config, original config, working directory, and root working directory; the download
// directory, source, and source map; the tofu command, original command, and CLI args; the IAM role options and their
// original values; the feature flag overrides; the auth provider command; the parser settings; the
// read_terragrunt_config chain; SkipOutput; SkipOutputsResolution; the parse depth; SkipAutoIncludeMerge; whether
// reads are tracked; and whether the parse is partial.
//
// It leaves out the experiments, the strict controls, the maximum number of parent folders to check, the engine,
// the tofu binary and implementation, the provider cache and CAS options, the environment, and the telemetry and
// logging options.
//
// The parse runs unmemoized for a supplied `dependency`, values, feature, or locals value, a supplied include
// list, and a diagnostics handler.
func runDigest(run *pkgconfig.ParsingContext) (string, bool) {
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
	d.bool(run.FilesRead.Tracking())
	d.bool(len(run.PartialParseDecodeList) > 0)

	return d.sum(), true
}

// extend returns the digest of key followed by parts.
func extend(key string, parts ...string) string {
	d := newDigest()
	d.str(key)
	d.strs(parts)

	return d.sum()
}

// digest accumulates typed parts into one hash, each followed by a separator.
type digest struct {
	h hash.Hash
}

func newDigest() digest {
	return digest{h: sha256.New()}
}

func (d digest) str(s string) {
	d.h.Write([]byte(s))
	d.h.Write([]byte{0})
}

func (d digest) strs(ss []string) {
	d.int(len(ss))

	for _, s := range ss {
		d.str(s)
	}
}

func (d digest) strMap(m map[string]string) {
	d.int(len(m))

	for _, k := range slices.Sorted(maps.Keys(m)) {
		d.str(k)
		d.str(m[k])
	}
}

func (d digest) bool(b bool) {
	d.str(strconv.FormatBool(b))
}

func (d digest) int(n int) {
	d.str(strconv.Itoa(n))
}

func (d digest) roleOptions(o iam.RoleOptions) {
	d.str(o.RoleARN)
	d.str(o.WebIdentityToken)
	d.str(o.AssumeRoleSessionName)
	d.str(strconv.FormatInt(o.AssumeRoleDuration, 10))
}

func (d digest) parserSettings(s pkgconfig.ParserSettings) {
	d.int(int(s.Diagnostics))
	d.bool(s.RewriteBareInclude)
	d.bool(s.IgnoreDiagnostics)
	d.bool(s.SkipDefaults)
	d.strs(s.HaltOnErrorOnlyInBlocks)
}

func (d digest) sum() string {
	return hex.EncodeToString(d.h.Sum(nil))
}
