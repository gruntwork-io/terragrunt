// FuzzFullCLI drives the entire Terragrunt CLI through the in-memory venv
// bundle (vfs/vexec/vhttp/env/writers). A single fuzz-input byte slice
// deterministically produces the args, environment, filesystem seed, and
// subprocess/HTTP responses for one iteration.
//
// Failure signals: panics, data races (under -race), and a non-nil
// RunContext error that produces empty stderr even after the standard
// post-RunContext logging that main.go performs in production.
//
// Inputs are shaped by a small grammar:
//
//  1. A uint16 selects one of N argSpecs — a (subcommand, flag pool,
//     TF-passthrough) tuple modelling a structurally valid invocation
//     (e.g., "run --all -- plan -input=false").
//  2. A uint16 selects one of M fsShapes — a coherent file layout (simple
//     unit, unit-with-dep, stack, include chain, errors block, auto-include,
//     malformed). Each shape lays down canonical files and substitutes
//     fuzz-driven bytes into one or two slots for variability.
//  3. Remaining bytes drive flag values, env entries, subprocess responses,
//     and HTTP responses.
//
// Known sources of cross-run noise (do not affect any failure signal):
//
//  1. math/rand and crypto/rand calls produce per-run variation in output
//     strings — session-name suffixes, generated IDs.
//  2. A handful of os.Getenv/os.Setenv sites remain in shell-completion,
//     help-debug, and config_helpers lock paths. The arg pool excludes
//     the tokens reaching the first two; the third is internal and harmless.
//  3. Leaf utility code in git/hash/lockfile helpers calls os.* directly.
//     If reached, those calls touch real disk; the per-iteration context
//     timeout bounds the blast radius.
package test_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// fuzzPerRunTimeout caps real-clock work in a single iteration so
	// retry/backoff loops do not stall a fuzz worker indefinitely.
	fuzzPerRunTimeout = 5 * time.Second
	// fuzzSlowThreshold is the wall-clock budget for a single RunContext
	// call. Iterations slower than this are reported as failures so the
	// fuzz framework can minimize toward a reproducer; otherwise they
	// exceed the framework's worker timeout and produce opaque EOFs that
	// minimization can't unwind. The typical seed iteration is under 100ms,
	// so a 2-second budget leaves ~20x headroom for legitimately slow paths.
	fuzzSlowThreshold    = 2 * time.Second
	fuzzWorkDir          = "/work"
	fuzzMaxFlagsPerCmd   = 4
	fuzzMaxTFArgs        = 3
	fuzzMaxEnvEntries    = 8
	fuzzMaxFuzzSlotChars = 24
	fuzzFileMode         = 0o644
	fuzzMaxExecOutLen    = 64
	fuzzMaxHTTPBodyLen   = 128
)

// flagTpl renders a CLI flag with an optional value chosen from the fuzz
// stream. A nil values slice means the flag is a boolean (no value).
type flagTpl struct {
	name   string
	values []string
}

func (f flagTpl) render(c *consumer) string {
	if len(f.values) == 0 {
		return f.name
	}

	return f.name + "=" + c.choose(f.values)
}

// argSpec models a structurally valid Terragrunt invocation: a head
// subcommand, an optional second token, a pool of flags appropriate for
// that subcommand, and a hint that TF passthrough args may follow `--`.
type argSpec struct {
	head   string
	sub    string
	flags  []flagTpl
	tfArgs bool
}

// fuzzGlobalFlags are accepted on every subcommand. They cover logging,
// strict mode, experiments, and the working-dir override. Values are kept
// small so libFuzzer can mutate them within the curated set.
var fuzzGlobalFlags = []flagTpl{
	{name: "--non-interactive"},
	{name: "--no-color"},
	{name: "--log-level", values: []string{"trace", "debug", "info", "warn", "error"}},
	{name: "--log-format", values: []string{"bare", "key-value", "json", "pretty"}},
	{name: "--log-disable"},
	{name: "--log-show-abs-paths"},
	{name: "--strict-mode"},
	{name: "--strict-control", values: []string{
		"deprecated-aws-getter", "skip-dependencies-inputs",
		"deprecated-commands", "deprecated-flags", "deprecated-env-vars",
		"deprecated-configs", "terragrunt-prefix-flags", "terragrunt-prefix-env-vars",
		"bare-include", "root-terragrunt-hcl", "legacy-internal-tflint",
		"deprecated-hidden-flag", "queue-strict-include", "cli-redesign",
	}},
	{name: "--experiment-mode"},
	{name: "--experiment", values: []string{
		"cli-redesign", "stack", "auto-init",
		"symlinks", "cas", "report", "iac-engine",
		"dependency-fetch-output-from-state", "slow-task-reporting",
		"dag-queue-display", "stack-dependencies", "catalog-redesign", "runner-pool",
	}},
	{name: "--no-tips"},
	{name: "--working-dir", values: []string{fuzzWorkDir, fuzzWorkDir + "/app", fuzzWorkDir + "/db", fuzzWorkDir + "/units/foo"}},
}

var fuzzRunFlags = append([]flagTpl{
	{name: "--all"},
	{name: "--graph"},
	{name: "--no-auto-init"},
	{name: "--no-auto-retry"},
	{name: "--no-auto-approve"},
	{name: "--source", values: []string{"./mod", fuzzWorkDir + "/mod"}},
	{name: "--source-update"},
	{name: "--source-map", values: []string{"github.com/a=github.com/b"}},
	{name: "--queue-include-external"},
	{name: "--queue-exclude-external"},
	{name: "--queue-include-dir", values: []string{fuzzWorkDir + "/app", fuzzWorkDir + "/db"}},
	{name: "--queue-exclude-dir", values: []string{fuzzWorkDir + "/db"}},
	{name: "--queue-ignore-errors"},
	{name: "--queue-ignore-dag-order"},
	{name: "--queue-construct-as", values: []string{"apply", "plan", "destroy"}},
	{name: "--queue-include-units-reading", values: []string{fuzzWorkDir + "/root.hcl"}},
	{name: "--report-format", values: []string{"json", "csv"}},
	{name: "--report-file", values: []string{fuzzWorkDir + "/report.json"}},
	{name: "--summary-disable"},
	{name: "--summary-per-unit"},
	{name: "--parallelism", values: []string{"1", "4", "16"}},
	{name: "--fail-fast"},
	{name: "--tf-forward-stdout"},
	{name: "--no-destroy-dependencies-check"},
	{name: "--use-partial-parse-config-cache"},
	{name: "--feature", values: []string{"name=value", "x=true", "env=dev"}},
	{name: "--auth-provider-cmd", values: []string{"echo {}"}},
	{name: "--iam-assume-role", values: []string{"arn:aws:iam::123456789012:role/r"}},
	{name: "--iam-assume-role-duration", values: []string{"3600"}},
	{name: "--provider-cache"},
	{name: "--provider-cache-dir", values: []string{fuzzWorkDir + "/pcache"}},
	{name: "--provider-cache-port", values: []string{"0"}},
	{name: "--config", values: []string{fuzzWorkDir + "/terragrunt.hcl"}},
	{name: "--download-dir", values: []string{fuzzWorkDir + "/.cache"}},
	{name: "--tf-path", values: []string{"/usr/bin/tofu", "/usr/bin/terraform"}},
	{name: "--filter", values: []string{"*", "app", "!db"}},
	{name: "--inputs-debug"},
	{name: "--out-dir", values: []string{fuzzWorkDir + "/out"}},
	{name: "--json-out-dir", values: []string{fuzzWorkDir + "/jout"}},
}, fuzzGlobalFlags...)

var fuzzStackFlags = append([]flagTpl{
	{name: "--format", values: []string{"json", "text"}},
	{name: "--no-stack-validate"},
	{name: "--no-stack-generate"},
}, fuzzGlobalFlags...)

var fuzzFindFlags = append([]flagTpl{
	{name: "--json"},
	{name: "--hidden"},
	{name: "--dependencies"},
	{name: "--external"},
	{name: "--sort", values: []string{"alpha", "dag"}},
	{name: "--queue-construct-as", values: []string{"apply", "plan", "destroy"}},
	{name: "--filter", values: []string{"*", "app", "!db"}},
}, fuzzGlobalFlags...)

var fuzzListFlags = append([]flagTpl{
	{name: "--long"},
	{name: "--tree"},
	{name: "--group-by", values: []string{"type", "dag"}},
	{name: "--json"},
	{name: "--sort", values: []string{"alpha", "dag"}},
	{name: "--queue-construct-as", values: []string{"apply", "plan", "destroy"}},
	{name: "--filter", values: []string{"*", "app", "!db"}},
}, fuzzGlobalFlags...)

var fuzzHCLFlags = append([]flagTpl{
	{name: "--check"},
	{name: "--diff"},
	{name: "--exclude-dir", values: []string{".terragrunt-cache"}},
	{name: "--file", values: []string{fuzzWorkDir + "/terragrunt.hcl"}},
}, fuzzGlobalFlags...)

var fuzzRenderFlags = append([]flagTpl{
	{name: "--format", values: []string{"json", "hcl"}},
	{name: "--with-metadata"},
	{name: "--out", values: []string{fuzzWorkDir + "/render.json"}},
}, fuzzGlobalFlags...)

var fuzzBackendFlags = append([]flagTpl{
	{name: "--backend-bootstrap"},
	{name: "--backend-require-bootstrap"},
	{name: "--disable-bucket-update"},
	{name: "--force"},
}, fuzzGlobalFlags...)

var fuzzCatalogFlags = append([]flagTpl{
	{name: "--no-shell"},
	{name: "--no-hooks"},
}, fuzzGlobalFlags...)

// fuzzArgSpecs enumerates the shape of every invocation the fuzz can
// produce. The grammar guarantees that most iterations parse past CLI
// validation and reach the subcommand's body; raw random tokens are
// excluded so iteration time isn't burned on flag-parser error messages.
var fuzzArgSpecs = []argSpec{
	{head: "version"},
	{head: "info", flags: fuzzGlobalFlags},
	{head: "info", sub: "print", flags: fuzzGlobalFlags},
	{head: "info", sub: "strict", flags: fuzzGlobalFlags},
	{head: "find", flags: fuzzFindFlags},
	{head: "list", flags: fuzzListFlags},
	{head: "run", flags: fuzzRunFlags, tfArgs: true},
	{head: "run", sub: "apply", flags: fuzzRunFlags, tfArgs: true},
	{head: "stack", sub: "generate", flags: fuzzStackFlags},
	{head: "stack", sub: "run", flags: fuzzStackFlags, tfArgs: true},
	{head: "stack", sub: "output", flags: fuzzStackFlags},
	{head: "stack", sub: "clean", flags: fuzzStackFlags},
	{head: "hcl", sub: "format", flags: fuzzHCLFlags},
	{head: "hcl", sub: "validate", flags: fuzzHCLFlags},
	{head: "dag", sub: "graph", flags: fuzzGlobalFlags},
	{head: "render", flags: fuzzRenderFlags},
	{head: "exec", flags: fuzzGlobalFlags, tfArgs: true},
	{head: "apply", flags: fuzzGlobalFlags, tfArgs: true},
	{head: "plan", flags: fuzzGlobalFlags, tfArgs: true},
	{head: "destroy", flags: fuzzGlobalFlags, tfArgs: true},
	{head: "init", flags: fuzzGlobalFlags},
	{head: "validate", flags: fuzzGlobalFlags},
	{head: "output", flags: fuzzGlobalFlags},
	{head: "show", flags: fuzzGlobalFlags},
	{head: "fmt", flags: fuzzGlobalFlags},
	{head: "backend", sub: "bootstrap", flags: fuzzBackendFlags},
	{head: "backend", sub: "migrate", flags: fuzzBackendFlags},
	{head: "backend", sub: "delete", flags: fuzzBackendFlags},
	{head: "catalog", flags: fuzzCatalogFlags},
}

// fuzzTFPassthroughPool is the alphabet of args sampled after `--` to feed
// the (in-memory) Terraform/OpenTofu subprocess. These are inert under
// vexec, but they exercise Terragrunt's arg-rewriting and detection logic.
var fuzzTFPassthroughPool = []string{
	"-out=plan.bin",
	"-input=false",
	"-no-color",
	"-auto-approve",
	"-refresh=false",
	"-target=null_resource.foo",
	"-detailed-exitcode",
	"-lock=false",
}

// fuzzEnvKeyPool seeds Venv.Env with names the codebase reads (Terragrunt
// flags plus cloud-provider variables the SDK config builders consume).
var fuzzEnvKeyPool = []string{
	"TG_LOG_LEVEL", "TG_LOG_FORMAT", "TG_NON_INTERACTIVE",
	"TG_WORKING_DIR", "TG_EXPERIMENT_MODE", "TG_STRICT_MODE",
	"TG_NO_COLOR", "TG_LOG_DISABLE",
	"TG_SOURCE", "TG_SOURCE_UPDATE", "TG_SOURCE_MAP",
	"TG_FEATURE", "TG_AUTH_PROVIDER_CMD",
	"TG_IAM_ASSUME_ROLE", "TG_IAM_ASSUME_ROLE_DURATION",
	"TG_PROVIDER_CACHE", "TG_PROVIDER_CACHE_DIR",
	"TG_FILTER", "TG_PARALLELISM", "TG_FAIL_FAST",
	"TG_TF_PATH", "TG_DOWNLOAD_DIR", "TG_CONFIG",
	"TG_USE_PARTIAL_PARSE_CONFIG_CACHE",
	"TG_QUEUE_IGNORE_ERRORS", "TG_QUEUE_IGNORE_DAG_ORDER",
	"TG_QUEUE_CONSTRUCT_AS",
	"TG_INPUTS_DEBUG", "TG_TF_FORWARD_STDOUT",
	"TG_BACKEND_BOOTSTRAP",
	"TERRAGRUNT_LOG_LEVEL", "TERRAGRUNT_DEBUG", "TERRAGRUNT_WORKING_DIR",
	"TERRAGRUNT_SOURCE", "TERRAGRUNT_AUTH_PROVIDER_CMD",
	"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_PROJECT",
	"HOME", "PATH", "USER", "TMPDIR",
	"TF_VAR_region", "TF_PLUGIN_CACHE_DIR",
}

// consumer reads structured values from a fuzz-input byte slice. Accesses
// are mutex-guarded because subprocess and HTTP handlers built from the
// same consumer may be invoked concurrently by Terragrunt's parallel unit
// runner. When the underlying reader is exhausted, every accessor returns
// the zero value so tiny inputs still produce a valid world.
type consumer struct {
	r  *bytes.Reader
	mu sync.Mutex
}

func newConsumer(data []byte) *consumer {
	return &consumer{r: bytes.NewReader(data)}
}

func (c *consumer) byteOnce() byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := c.r.ReadByte()
	if err != nil {
		return 0
	}

	return b
}

func (c *consumer) uint16() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var buf [2]byte

	n, err := c.r.Read(buf[:])
	if err != nil || n < len(buf) {
		return 0
	}

	return binary.LittleEndian.Uint16(buf[:])
}

func (c *consumer) intN(n int) int {
	if n <= 0 {
		return 0
	}

	return int(c.uint16()) % n
}

func (c *consumer) boolean() bool {
	return c.byteOnce()&1 == 1
}

// slot returns a short alphabet-restricted string of length 0..maxLen.
// Used to fill the small fuzz-driven slots inside otherwise-canonical
// HCL fixtures (locals values, attribute names, paths).
func (c *consumer) slot(maxLen int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz_0123456789"

	n := c.intN(maxLen + 1)
	if n == 0 {
		return ""
	}

	buf := make([]byte, n)
	for i := range buf {
		buf[i] = alphabet[int(c.byteOnce())%len(alphabet)]
	}

	return string(buf)
}

func (c *consumer) choose(pool []string) string {
	if len(pool) == 0 {
		return ""
	}

	return pool[c.intN(len(pool))]
}

type fuzzWorld struct {
	env    map[string]string
	args   []string
	fsSeed []fuzzSeedFile
}

type fuzzSeedFile struct {
	path string
	data []byte
}

func deriveFuzzWorld(c *consumer) fuzzWorld {
	return fuzzWorld{
		args:   buildFuzzArgs(c),
		fsSeed: buildFuzzFS(c),
		env:    buildFuzzEnv(c),
	}
}

func buildFuzzArgs(c *consumer) []string {
	spec := fuzzArgSpecs[c.intN(len(fuzzArgSpecs))]

	args := []string{spec.head}
	if spec.sub != "" {
		args = append(args, spec.sub)
	}

	flagCount := c.intN(fuzzMaxFlagsPerCmd + 1)
	for range flagCount {
		if len(spec.flags) == 0 {
			break
		}

		args = append(args, spec.flags[c.intN(len(spec.flags))].render(c))
	}

	if spec.tfArgs && c.boolean() {
		args = append(args, "--")

		tfCount := c.intN(fuzzMaxTFArgs + 1)
		for range tfCount {
			args = append(args, c.choose(fuzzTFPassthroughPool))
		}
	}

	return args
}

func buildFuzzEnv(c *consumer) map[string]string {
	env := map[string]string{}

	count := c.intN(fuzzMaxEnvEntries + 1)
	for range count {
		key := c.choose(fuzzEnvKeyPool)
		if key == "" {
			continue
		}

		env[key] = c.slot(fuzzMaxFuzzSlotChars)
	}

	return env
}

// fuzzShape lays down one of several coherent FS layouts. Each shape
// targets a distinct path through Terragrunt's discovery/config/runner
// pipeline. Fuzz-driven slots inside the canonical fixtures provide just
// enough variability for libFuzzer to find new coverage.
type fuzzShape func(c *consumer) []fuzzSeedFile

var fuzzShapes = []fuzzShape{
	fuzzShapeSimpleUnit,
	fuzzShapeWithDep,
	fuzzShapeStack,
	fuzzShapeIncludeChain,
	fuzzShapeErrorsBlock,
	fuzzShapeAutoInclude,
	fuzzShapeMalformed,
	fuzzShapeRemoteState,
	fuzzShapeGenerate,
	fuzzShapeFeatureFlags,
	fuzzShapeHooks,
	fuzzShapeRunCmd,
	fuzzShapeExclude,
	fuzzShapeErrorsIgnore,
	fuzzShapeExtraArgs,
	fuzzShapeStackNested,
	fuzzShapeDependenciesPaths,
}

func buildFuzzFS(c *consumer) []fuzzSeedFile {
	return fuzzShapes[c.intN(len(fuzzShapes))](c)
}

func fuzzShapeSimpleUnit(c *consumer) []fuzzSeedFile {
	name := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz")
	hcl := fmt.Sprintf(`
locals {
  name = %q
}

terraform {
  source = "./mod"
}

inputs = {
  name = local.name
}
`, name)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`
variable "name" { type = string }
resource "null_resource" "x" {}
output "id" { value = "fixed" }
`)},
	}
}

func fuzzShapeWithDep(c *consumer) []fuzzSeedFile {
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: fmt.Appendf(nil, `
locals {
  region = %q
}
`, region)},
		{path: fuzzWorkDir + "/db/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../mod"
}

inputs = {
  region = include.root.locals.region
}
`)},
		{path: fuzzWorkDir + "/app/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "db" {
  config_path = "../db"

  mock_outputs = {
    id = "mock-id"
  }
}

terraform {
  source = "../mod"
}

inputs = {
  db_id = dependency.db.outputs.id
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`
resource "null_resource" "x" {}
output "id" { value = "fixed" }
`)},
	}
}

func fuzzShapeStack(c *consumer) []fuzzSeedFile {
	unitName := orDefault(c.slot(fuzzMaxFuzzSlotChars), "foo")
	stack := fmt.Sprintf(`
unit %q {
  source = "./units/%s"
  path   = "live/%s"
}

unit "bar" {
  source = "./units/bar"
  path   = "live/bar"
}
`, unitName, unitName, unitName)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: []byte(`locals { region = "us-east-1" }`)},
		{path: fuzzWorkDir + "/terragrunt.stack.hcl", data: []byte(stack)},
		{path: fuzzWorkDir + "/units/" + unitName + "/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../mod"
}
`)},
		{path: fuzzWorkDir + "/units/bar/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../mod"
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func fuzzShapeIncludeChain(c *consumer) []fuzzSeedFile {
	envName := orDefault(c.slot(fuzzMaxFuzzSlotChars), "dev")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: fmt.Appendf(nil, `
locals {
  env = %q
}
`, envName)},
		{path: fuzzWorkDir + "/sub/region.hcl", data: []byte(`
locals {
  region = "us-east-1"
}
`)},
		{path: fuzzWorkDir + "/sub/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include "region" {
  path = "region.hcl"
}

terraform {
  source = "../mod"
}

inputs = {
  env    = include.root.locals.env
  region = include.region.locals.region
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func fuzzShapeErrorsBlock(c *consumer) []fuzzSeedFile {
	maxAttempts := 1 + c.intN(4)
	hcl := fmt.Sprintf(`
terraform {
  source = "./mod"
}

errors {
  retry "transient" {
    retryable_errors = [".*timeout.*", ".*rate limit.*"]
    max_attempts     = %d
    sleep_interval_sec = 1
  }
}

inputs = {
  name = "fuzz"
}
`, maxAttempts)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func fuzzShapeAutoInclude(c *consumer) []fuzzSeedFile {
	regionLocal := orDefault(c.slot(fuzzMaxFuzzSlotChars), "eu-west-1")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.autoinclude.hcl", data: fmt.Appendf(nil, `
locals {
  region = %q
}
`, regionLocal)},
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(`
terraform {
  source = "./mod"
}

inputs = {
  region = "us-east-1"
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeMalformed seeds the workdir with random fuzz bytes for the
// HCL files. This is the only shape where the parser sees genuinely
// arbitrary input; the others bias toward well-formed configs so the
// fuzzer reaches code below the parser.
func fuzzShapeMalformed(c *consumer) []fuzzSeedFile {
	tg := c.slot(fuzzMaxFuzzSlotChars * 8)
	root := c.slot(fuzzMaxFuzzSlotChars * 8)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(tg)},
		{path: fuzzWorkDir + "/root.hcl", data: []byte(root)},
	}
}

// fuzzShapeRemoteState drives the remotestate package and the AWS/GCS
// backend builders. Backend transport routes through v.HTTP, so calls
// are bounded by the fuzz HTTP handler rather than real network.
func fuzzShapeRemoteState(c *consumer) []fuzzSeedFile {
	backend := c.choose([]string{"s3", "gcs", "local"})
	bucket := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz-bucket")
	key := orDefault(c.slot(fuzzMaxFuzzSlotChars), "tfstate")
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")
	hcl := fmt.Sprintf(`
remote_state {
  backend                         = %q
  disable_init                    = false
  disable_dependency_optimization = false
  config = {
    bucket  = %q
    key     = %q
    region  = %q
    encrypt = true
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

terraform {
  source = "./mod"
}

inputs = {
  region = %q
}
`, backend, bucket, key, region, region)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeGenerate exercises codegen's contents/signature/if_exists/comment
// preparation. The final os.WriteFile under /work fails fast on macOS/CI
// (parent missing), so only the pre-write logic is covered.
func fuzzShapeGenerate(c *consumer) []fuzzSeedFile {
	name := orDefault(c.slot(fuzzMaxFuzzSlotChars), "provider")
	contents := orDefault(c.slot(fuzzMaxFuzzSlotChars*4), `provider "null" {}`)
	ifExists := c.choose([]string{"overwrite", "overwrite_terragrunt", "skip"})
	commentPrefix := orDefault(c.slot(fuzzMaxFuzzSlotChars), "# ")
	hcl := fmt.Sprintf(`
generate %q {
  path              = "%s.tf"
  contents          = %q
  if_exists         = %q
  comment_prefix    = %q
  disable_signature = false
}

generate "backend" {
  path      = "backend.tf"
  contents  = "terraform { backend \"local\" {} }"
  if_exists = "overwrite"
}

terraform {
  source = "./mod"
}
`, name, name, contents, ifExists, commentPrefix)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeFeatureFlags drives feature-flag parse and substitution in
// inputs evaluation.
func fuzzShapeFeatureFlags(c *consumer) []fuzzSeedFile {
	feat := orDefault(c.slot(fuzzMaxFuzzSlotChars), "feat")
	defVal := orDefault(c.slot(fuzzMaxFuzzSlotChars), "default")
	hcl := fmt.Sprintf(`
feature %q {
  default = %q
}

terraform {
  source = "./mod"
}

inputs = {
  x = feature.%s.value
}
`, feat, defVal, feat)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeHooks routes before/after/error hook execution through v.Exec
// via shell.RunCommandWithOutput. extra_arguments exercises arg-injection.
func fuzzShapeHooks(c *consumer) []fuzzSeedFile {
	cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
	arg := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz")
	runOnError := c.boolean()
	hcl := fmt.Sprintf(`
terraform {
  source = "./mod"

  before_hook "pre" {
    commands = [%q]
    execute  = ["echo", %q]
  }

  after_hook "post" {
    commands     = [%q]
    execute      = ["echo", "after"]
    run_on_error = %t
  }

  error_hook "err" {
    commands  = [%q]
    on_errors = [".*"]
    execute   = ["echo", "err"]
  }

  extra_arguments "extra" {
    commands  = [%q]
    arguments = ["-var", "x=1"]
    env_vars  = {
      FUZZ = %q
    }
  }
}
`, cmd, arg, cmd, runOnError, cmd, cmd, arg)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeRunCmd exercises the run_cmd HCL function, which routes
// subprocesses through v.Exec.
func fuzzShapeRunCmd(c *consumer) []fuzzSeedFile {
	msg := orDefault(c.slot(fuzzMaxFuzzSlotChars), "hello")
	hcl := fmt.Sprintf(`
locals {
  greeting = run_cmd("--terragrunt-quiet", "echo", %q)
}

terraform {
  source = "./mod"
}

inputs = {
  message = local.greeting
}
`, msg)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeExclude exercises the exclude block evaluation paths.
func fuzzShapeExclude(c *consumer) []fuzzSeedFile {
	action := c.choose([]string{"apply", "plan", "destroy", "all"})
	noRun := c.boolean()
	excludeDeps := c.boolean()
	hcl := fmt.Sprintf(`
exclude {
  if                   = true
  actions              = [%q]
  no_run               = %t
  exclude_dependencies = %t
}

terraform {
  source = "./mod"
}
`, action, noRun, excludeDeps)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeErrorsIgnore exercises errors { ignore "x" { ... } } alongside
// the retry variant covered by fuzzShapeErrorsBlock.
func fuzzShapeErrorsIgnore(c *consumer) []fuzzSeedFile {
	pattern := orDefault(c.slot(fuzzMaxFuzzSlotChars), "ignored")
	sigVal := orDefault(c.slot(fuzzMaxFuzzSlotChars), "sigval")
	msg := orDefault(c.slot(fuzzMaxFuzzSlotChars), "ignored error")
	hcl := fmt.Sprintf(`
errors {
  retry "transient" {
    retryable_errors   = [".*timeout.*"]
    max_attempts       = 2
    sleep_interval_sec = 1
  }
  ignore "known" {
    ignorable_errors = [%q]
    message          = %q
    signals = {
      foo = %q
    }
  }
}

terraform {
  source = "./mod"
}
`, pattern, msg, sigVal)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeExtraArgs exercises the extra_arguments arg-injection path.
func fuzzShapeExtraArgs(c *consumer) []fuzzSeedFile {
	cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
	val := orDefault(c.slot(fuzzMaxFuzzSlotChars), "v")
	hcl := fmt.Sprintf(`
terraform {
  source = "./mod"

  extra_arguments "vars" {
    commands  = [%q]
    arguments = ["-var", "x=%s"]
    env_vars = {
      FUZZ = %q
    }
    required_var_files = []
  }
}
`, cmd, val, val)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeStackNested exercises stack-level locals, include, values, and
// nested stack-of-stacks composition.
func fuzzShapeStackNested(c *consumer) []fuzzSeedFile {
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")
	childName := orDefault(c.slot(fuzzMaxFuzzSlotChars), "child")
	stackHCL := fmt.Sprintf(`
locals {
  region = %q
}

unit "a" {
  source = "./units/a"
  path   = "live/a"
  values = {
    region = local.region
  }
}

stack %q {
  source = "./stacks/child"
  path   = "child"
}
`, region, childName)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: []byte(`locals { region = "us-east-1" }`)},
		{path: fuzzWorkDir + "/terragrunt.stack.hcl", data: []byte(stackHCL)},
		{path: fuzzWorkDir + "/units/a/terragrunt.hcl", data: []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../mod"
}
`)},
		{path: fuzzWorkDir + "/stacks/child/terragrunt.stack.hcl", data: []byte(`
unit "leaf" {
  source = "./units/leaf"
  path   = "live/leaf"
}
`)},
		{path: fuzzWorkDir + "/stacks/child/units/leaf/terragrunt.hcl", data: []byte(`
terraform {
  source = "../../../mod"
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeDependenciesPaths exercises the plural-form `dependencies` block
// distinct from the singular `dependency "X"` form covered by fuzzShapeWithDep.
func fuzzShapeDependenciesPaths(c *consumer) []fuzzSeedFile {
	extra := orDefault(c.slot(fuzzMaxFuzzSlotChars), "other")
	appHCL := fmt.Sprintf(`
dependencies {
  paths = ["../db", "../%s"]
}

terraform {
  source = "../mod"
}
`, extra)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/db/terragrunt.hcl", data: []byte(`
terraform {
  source = "../mod"
}
`)},
		{path: fuzzWorkDir + "/" + extra + "/terragrunt.hcl", data: []byte(`
terraform {
  source = "../mod"
}
`)},
		{path: fuzzWorkDir + "/app/terragrunt.hcl", data: []byte(appHCL)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

// fuzzExecHandler dispatches every in-memory subprocess invocation through
// c, so the response is fuzz-driven and reproducible for a given input.
// Concurrent invocations serialize on the consumer's mutex.
func fuzzExecHandler(c *consumer) vexec.Handler {
	return func(_ context.Context, _ vexec.Invocation) vexec.Result {
		exit := int(c.byteOnce() % 4)
		if exit == 3 {
			exit = 0
		}

		return vexec.Result{
			ExitCode: exit,
			Stdout:   []byte(c.slot(fuzzMaxExecOutLen)),
			Stderr:   []byte(c.slot(fuzzMaxExecOutLen)),
		}
	}
}

// fuzzHTTPHandler synthesizes responses for every outbound HTTP request.
// AWS and GCP SDKs route through the same handler via vhttp; bias toward
// "{}" so JSON decoders exercise downstream code instead of bailing.
func fuzzHTTPHandler(c *consumer) vhttp.Handler {
	return func(_ context.Context, _ *http.Request) (*http.Response, error) {
		status := 200 + int(c.byteOnce()%3)*100

		body := []byte("{}")
		if c.boolean() {
			body = []byte(c.slot(fuzzMaxHTTPBodyLen))
		}

		return vhttp.Respond(status, body, nil), nil
	}
}

func fuzzLookPath() vexec.PathHandler {
	return func(file string) (string, error) {
		return "/usr/bin/" + file, nil
	}
}

func FuzzFullCLI(f *testing.F) {
	// Seeds bias libFuzzer's initial exploration toward each subcommand
	// and FS shape. The exact bytes don't need to encode any particular
	// world; they are starting points for coverage-guided mutation.
	f.Add([]byte{})
	f.Add([]byte("version"))
	f.Add([]byte("info print"))
	f.Add([]byte("info strict"))
	f.Add([]byte("find --json --dependencies"))
	f.Add([]byte("list --tree"))
	f.Add([]byte("run --all -- plan -input=false"))
	f.Add([]byte("run apply -- -auto-approve"))
	f.Add([]byte("stack generate"))
	f.Add([]byte("stack run -- apply"))
	f.Add([]byte("stack output --format=json"))
	f.Add([]byte("stack clean"))
	f.Add([]byte("hcl format --check"))
	f.Add([]byte("hcl validate"))
	f.Add([]byte("dag graph"))
	f.Add([]byte("render --format=json"))
	f.Add([]byte("apply -- -auto-approve"))
	f.Add([]byte("plan -- -out=plan.bin"))
	f.Add([]byte("init"))
	f.Add([]byte("backend bootstrap"))
	f.Add([]byte("backend migrate"))
	f.Add([]byte("backend delete --force"))
	f.Add([]byte("catalog --no-shell --no-hooks"))
	f.Add([]byte("run --all --feature foo=bar -- apply"))
	f.Add([]byte("run --filter app --parallelism=4 -- plan"))
	f.Add([]byte("run --provider-cache --auth-provider-cmd=echo"))
	f.Add([]byte("run --iam-assume-role=arn:aws:iam::1:role/r -- plan"))
	f.Add([]byte("run --queue-ignore-errors --queue-construct-as=destroy"))
	f.Add([]byte("run --summary-per-unit --tf-forward-stdout"))
	f.Add([]byte("find --filter '*' --queue-construct-as=plan"))
	f.Add([]byte("list --filter '!db' --sort=dag"))
	f.Add([]byte("run --inputs-debug --use-partial-parse-config-cache"))
	f.Add(bytes.Repeat([]byte{0xab}, 64))

	f.Fuzz(func(t *testing.T, data []byte) {
		c := newConsumer(data)
		w := deriveFuzzWorld(c)

		out := &bytes.Buffer{}
		errBuf := &bytes.Buffer{}

		v := &venv.Venv{
			FS:      vfs.NewMemMapFS(),
			Exec:    vexec.NewMemExec(fuzzExecHandler(c), vexec.WithLookPath(fuzzLookPath())),
			HTTP:    vhttp.NewMemClient(fuzzHTTPHandler(c)),
			Env:     w.env,
			Writers: &writer.Writers{Writer: out, ErrWriter: errBuf},
		}

		for _, sf := range w.fsSeed {
			if err := vfs.WriteFile(v.FS, sf.path, sf.data, fuzzFileMode); err != nil {
				t.Fatalf("seed FS: %v", err)
			}
		}

		l := log.New(
			log.WithOutput(v.Writers.ErrWriter),
			log.WithLevel(options.DefaultLogLevel),
			log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
		)

		opts := options.NewTerragruntOptions()
		// Pin the working directory to the in-memory venv root so that
		// fuzz-generated invocations without --working-dir cannot fall
		// through to the test process's real CWD. Without this, Terragrunt
		// discovers fixtures under test/ on real disk and the run-all
		// path through copyFiles / go-getter (neither virtualized) chews
		// through them on every iteration.
		opts.WorkingDir = fuzzWorkDir
		app := cli.NewApp(l, opts, v)

		ctx, cancel := context.WithTimeout(t.Context(), fuzzPerRunTimeout)
		defer cancel()

		ctx = log.ContextWithLogger(ctx, l)

		// Every argSpec head in the fuzz grammar (find, list, version, info
		// print, backend bootstrap, ...) is part of the cli-redesign command
		// set. The legacy CLI doesn't recognize them and falls back to
		// printing the help text, so without these flags ~95% of iterations
		// bail before reaching discovery/run/etc. Force the experiment on
		// for every iteration; the grammar can still pick OTHER --experiment
		// values to layer on top.
		args := slices.Concat([]string{"--experiment-mode", "--experiment=cli-redesign"}, w.args)

		start := time.Now()
		err := app.RunContext(ctx, args)
		elapsed := time.Since(start)

		// Slow-iteration invariant: a single RunContext should finish well
		// under fuzzSlowThreshold on the in-memory venv. Iterations slower
		// than that point at code paths that ignore context cancellation
		// or perform work disproportionate to the input. Report the slowness
		// as a finding so the fuzz framework minimizes toward a reproducer;
		// otherwise the same iterations exceed the framework's worker
		// timeout and produce opaque EOF crashes that can't be minimized.
		if elapsed > fuzzSlowThreshold {
			t.Fatalf("iteration took %s (budget %s); ctx.Err()=%v; args=%q", elapsed, fuzzSlowThreshold, ctx.Err(), args)
		}

		// Mirror main.go's post-RunContext error display so the invariant
		// below checks the user-facing experience, not the internal
		// RunContext contract (RunContext returns errors but does not
		// display them; main.go logs them after it returns).
		if err != nil {
			l.Error(err.Error())
		}

		// Invariant: a non-nil error must produce some user-visible output
		// after the standard logging step. A silent failure here means the
		// error's Error() returned "" or the logger silently dropped the
		// message. Context-cancellation is exempt because the cancellation
		// is fuzz-harness-imposed.
		//
		// Exception: --log-disable runs an inline Setter during flag parsing
		// that silences the logger before any subsequent flag-parse error
		// has had a chance to surface. The post-RunContext l.Error call
		// then produces no output, and the user sees a silent non-zero
		// exit. We accept this here so the invariant focuses on more
		// interesting silent failures; the --log-disable / flag-parse
		// interaction is debatably a UX bug worth revisiting separately
		// (the principled fix is to apply the disable only after all
		// flags have parsed successfully).
		logDisabled := slices.Contains(args, "--log-disable")
		if err != nil && errBuf.Len() == 0 && ctx.Err() == nil && !logDisabled {
			t.Fatalf("RunContext returned %v with empty stderr after logging; args=%q", err, args)
		}
	})
}
