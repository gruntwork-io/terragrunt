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
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
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
	// catalog deliberately omitted: it launches a Bubble Tea TUI that
	// blocks waiting for terminal IO and clones repos from external git
	// remotes — neither is a productive fuzz surface.
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

// slotIdent returns a slot constrained to the HCL identifier shape:
// the first character is a letter or underscore, ruling out digit-led
// tokens that HCL parses as numeric literals (including scientific
// notation like `1e9999999`, which lands in `big.Float` with that many
// digits of precision and DoSes any later string conversion). Use this
// for slots emitted into unquoted positions — block labels in dotted
// traversals, identifier-like attribute names, etc. Quoted string slots
// can keep using slot().
func (c *consumer) slotIdent(maxLen int) string {
	const head = "abcdefghijklmnopqrstuvwxyz_"

	n := c.intN(maxLen + 1)
	if n == 0 {
		return ""
	}

	buf := make([]byte, n)
	buf[0] = head[int(c.byteOnce())%len(head)]

	if n > 1 {
		tail := c.slot(n - 1)
		copy(buf[1:], tail)

		for i := 1 + len(tail); i < n; i++ {
			buf[i] = '_'
		}
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
	fuzzShapeBuiltinFuncs,
	fuzzShapeMultiUnitRunAll,
	fuzzShapeExclude,
	fuzzShapeErrorsIgnore,
	fuzzShapeExtraArgs,
	fuzzShapeStackNested,
	fuzzShapeDependenciesPaths,
}

func buildFuzzFS(c *consumer) []fuzzSeedFile {
	return fuzzShapes[c.intN(len(fuzzShapes))](c)
}

// fuzzShapeSimpleUnit is the workhorse shape: a single unit at /work.
//
// The body is assembled with optional fragments rather than one fixed
// template so libFuzzer has structural flips to climb. Each c.boolean()
// below toggles a distinct code path (top-level attribute parse, hook
// dispatch, generate codegen, etc.). With the underlying byte stream
// exhausted, every flip defaults to false and the shape collapses back
// to the previous minimal form.
func fuzzShapeSimpleUnit(c *consumer) []fuzzSeedFile {
	name := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz")

	var b bytes.Buffer

	fmt.Fprintf(&b, "locals {\n  name = %q\n}\n\n", name)

	if c.boolean() {
		fmt.Fprintf(&b, "skip = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&b, "prevent_destroy = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&b, "download_dir = %q\n", c.choose([]string{fuzzWorkDir + "/.cache", fuzzWorkDir + "/dl"}))
	}

	if c.boolean() {
		role := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz")
		fmt.Fprintf(&b, "iam_role = %q\n", "arn:aws:iam::123456789012:role/"+role)
	}

	if c.boolean() {
		fmt.Fprintf(&b, "iam_assume_role_duration = %d\n", 900+c.intN(3000))
	}

	if c.boolean() {
		fmt.Fprintf(&b, "iam_assume_role_session_name = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz"))
	}

	if c.boolean() {
		fmt.Fprintf(&b, "terraform_version_constraint = %q\n", c.choose([]string{">= 1.0", ">= 0.13", "~> 1.5", "= 1.6.0"}))
	}

	if c.boolean() {
		fmt.Fprintf(&b, "terragrunt_version_constraint = %q\n", c.choose([]string{">= 0.50", "~> 0.60", "= 0.65.0"}))
	}

	if c.boolean() {
		feat := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "feat")
		defVal := c.choose([]string{`"default"`, `1`, `true`, `false`, `["a","b"]`})
		fmt.Fprintf(&b, "\nfeature %q {\n  default = %s\n}\n", feat, defVal)
	}

	if c.boolean() {
		ifExists := c.choose([]string{"overwrite", "overwrite_terragrunt", "skip"})
		fmt.Fprintf(&b, `
generate "backend" {
  path      = "backend.tf"
  contents  = "terraform { backend \"local\" {} }"
  if_exists = %q
}
`, ifExists)
	}

	b.WriteString("\nterraform {\n  source = \"./mod\"\n")

	if c.boolean() {
		b.WriteString("  copy_terraform_lock_file = true\n")
	}

	if c.boolean() {
		b.WriteString("  include_in_copy = [\"*.json\"]\n")
	}

	if c.boolean() {
		cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
		fmt.Fprintf(&b, "  before_hook \"pre\" {\n    commands = [%q]\n    execute  = [\"echo\", \"before\"]\n  }\n", cmd)
	}

	if c.boolean() {
		cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
		fmt.Fprintf(&b, "  after_hook \"post\" {\n    commands     = [%q]\n    execute      = [\"echo\", \"after\"]\n    run_on_error = %t\n  }\n", cmd, c.boolean())
	}

	if c.boolean() {
		cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
		fmt.Fprintf(&b, "  extra_arguments \"extra\" {\n    commands  = [%q]\n    arguments = [\"-var\", \"x=1\"]\n  }\n", cmd)
	}

	b.WriteString("}\n\ninputs = {\n  name = local.name\n")

	if c.boolean() {
		b.WriteString("  region = \"us-east-1\"\n")
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  count_val = %d\n", c.intN(100))
	}

	if c.boolean() {
		b.WriteString("  list_val = [1, 2, 3]\n")
	}

	if c.boolean() {
		b.WriteString("  nested = { a = 1, b = \"x\" }\n")
	}

	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`
variable "name" { type = string }
resource "null_resource" "x" {}
output "id" { value = "fixed" }
`)},
	}
}

// fuzzShapeWithDep exercises the dependency-block parse/eval depth. The
// dependency on /work/app -> /work/db is fixed (the include + dependency
// graph is what makes this shape distinct); the dependency block's
// attribute set varies with the fuzz stream so each evaluator branch
// (skip_outputs, enabled gating, merge strategy, allowed-commands list)
// becomes reachable.
func fuzzShapeWithDep(c *consumer) []fuzzSeedFile {
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")

	var dep bytes.Buffer

	dep.WriteString(`dependency "db" {
  config_path = "../db"
`)

	if c.boolean() {
		dep.WriteString("  mock_outputs = {\n    id = \"mock-id\"\n  }\n")
	}

	if c.boolean() {
		dep.WriteString("  mock_outputs_allowed_terraform_commands = [\"plan\", \"apply\", \"destroy\", \"init\", \"validate\"]\n")
	}

	if c.boolean() {
		fmt.Fprintf(&dep, "  mock_outputs_merge_strategy_with_state = %q\n", c.choose([]string{"no_merge", "shallow", "deep"}))
	}

	if c.boolean() {
		fmt.Fprintf(&dep, "  mock_outputs_merge_with_state = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&dep, "  skip_outputs = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&dep, "  enabled = %t\n", c.boolean())
	}

	dep.WriteString("}\n\n")

	var appHCL bytes.Buffer

	appHCL.WriteString(`include "root" {
  path = find_in_parent_folders("root.hcl")
`)

	if c.boolean() {
		fmt.Fprintf(&appHCL, "  merge_strategy = %q\n", c.choose([]string{"no_merge", "shallow", "deep"}))
	}

	if c.boolean() {
		fmt.Fprintf(&appHCL, "  expose = %t\n", c.boolean())
	}

	appHCL.WriteString("}\n\n")
	appHCL.Write(dep.Bytes())
	appHCL.WriteString("terraform {\n  source = \"../mod\"\n}\n\ninputs = {\n  db_id = dependency.db.outputs.id\n")

	if c.boolean() {
		appHCL.WriteString("  also = \"value\"\n")
	}

	appHCL.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: fmt.Appendf(nil, "locals {\n  region = %q\n}\n", region)},
		{path: fuzzWorkDir + "/db/terragrunt.hcl", data: []byte(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../mod"
}

inputs = {
  region = include.root.locals.region
}
`)},
		{path: fuzzWorkDir + "/app/terragrunt.hcl", data: appHCL.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}
output "id" { value = "fixed" }
`)},
	}
}

// fuzzShapeStack lays down a terragrunt.stack.hcl. Unit count, stack-level
// locals, per-unit values, and per-unit no_dot_terragrunt_stack all flip
// independently so the stack generator/runner branches each get exercised.
func fuzzShapeStack(c *consumer) []fuzzSeedFile {
	unitName := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "foo")

	var stack bytes.Buffer

	if c.boolean() {
		fmt.Fprintf(&stack, "locals {\n  region = %q\n  env    = %q\n}\n\n",
			orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1"),
			orDefault(c.slot(fuzzMaxFuzzSlotChars), "dev"))
	}

	fmt.Fprintf(&stack, "unit %q {\n  source = \"./units/%s\"\n  path   = \"live/%s\"\n",
		unitName, unitName, unitName)

	if c.boolean() {
		stack.WriteString("  values = {\n    region = \"us-east-1\"\n")

		if c.boolean() {
			fmt.Fprintf(&stack, "    name   = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "x"))
		}

		stack.WriteString("  }\n")
	}

	if c.boolean() {
		fmt.Fprintf(&stack, "  no_dot_terragrunt_stack = %t\n", c.boolean())
	}

	stack.WriteString("}\n\n")

	if c.boolean() {
		stack.WriteString("unit \"bar\" {\n  source = \"./units/bar\"\n  path   = \"live/bar\"\n")

		if c.boolean() {
			stack.WriteString("  values = { region = \"us-east-1\" }\n")
		}

		stack.WriteString("}\n")
	}

	files := []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: []byte(`locals { region = "us-east-1" }`)},
		{path: fuzzWorkDir + "/terragrunt.stack.hcl", data: stack.Bytes()},
		{path: fuzzWorkDir + "/units/" + unitName + "/terragrunt.hcl", data: []byte(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../mod"
}
`)},
		{path: fuzzWorkDir + "/units/bar/terragrunt.hcl", data: []byte(`include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../../mod"
}
`)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}

	return files
}

// fuzzShapeIncludeChain exercises the include merge/expose machinery. Two
// includes are always present (root + region) so the merge path is hit;
// merge_strategy and expose on each include flip independently.
func fuzzShapeIncludeChain(c *consumer) []fuzzSeedFile {
	envName := orDefault(c.slot(fuzzMaxFuzzSlotChars), "dev")

	var sub bytes.Buffer

	sub.WriteString("include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n")

	if c.boolean() {
		fmt.Fprintf(&sub, "  merge_strategy = %q\n", c.choose([]string{"no_merge", "shallow", "deep"}))
	}

	if c.boolean() {
		fmt.Fprintf(&sub, "  expose = %t\n", c.boolean())
	}

	sub.WriteString("}\n\ninclude \"region\" {\n  path = \"region.hcl\"\n")

	if c.boolean() {
		fmt.Fprintf(&sub, "  merge_strategy = %q\n", c.choose([]string{"no_merge", "shallow", "deep"}))
	}

	if c.boolean() {
		fmt.Fprintf(&sub, "  expose = %t\n", c.boolean())
	}

	sub.WriteString(`}

terraform {
  source = "../mod"
}

inputs = {
  env    = include.root.locals.env
  region = include.region.locals.region
`)

	if c.boolean() {
		sub.WriteString("  extra  = \"value\"\n")
	}

	sub.WriteString("}\n")

	root := bytes.Buffer{}
	fmt.Fprintf(&root, "locals {\n  env = %q\n", envName)

	if c.boolean() {
		fmt.Fprintf(&root, "  tier = %q\n", c.choose([]string{"prod", "staging", "dev"}))
	}

	root.WriteString("}\n")

	region := bytes.Buffer{}
	region.WriteString("locals {\n  region = \"us-east-1\"\n")

	if c.boolean() {
		region.WriteString("  zone = \"a\"\n")
	}

	region.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: root.Bytes()},
		{path: fuzzWorkDir + "/sub/region.hcl", data: region.Bytes()},
		{path: fuzzWorkDir + "/sub/terragrunt.hcl", data: sub.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func fuzzShapeErrorsBlock(c *consumer) []fuzzSeedFile {
	maxAttempts := 1 + c.intN(4)
	sleepSec := 1 + c.intN(5)

	var b bytes.Buffer

	b.WriteString("terraform {\n  source = \"./mod\"\n}\n\nerrors {\n")
	fmt.Fprintf(&b, "  retry \"transient\" {\n    retryable_errors   = [%q, %q]\n    max_attempts       = %d\n    sleep_interval_sec = %d\n  }\n",
		c.choose([]string{".*timeout.*", ".*throttl.*", ".*5xx.*", ".*EOF.*"}),
		c.choose([]string{".*rate limit.*", ".*deadline.*", ".*lock.*"}),
		maxAttempts, sleepSec)

	if c.boolean() {
		fmt.Fprintf(&b, "  retry \"network\" {\n    retryable_errors   = [\".*connection refused.*\"]\n    max_attempts       = %d\n    sleep_interval_sec = 1\n  }\n",
			1+c.intN(3))
	}

	b.WriteString("}\n\ninputs = {\n  name = \"fuzz\"\n")

	if c.boolean() {
		fmt.Fprintf(&b, "  attempt_cap = %d\n", maxAttempts)
	}

	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

func fuzzShapeAutoInclude(c *consumer) []fuzzSeedFile {
	regionLocal := orDefault(c.slot(fuzzMaxFuzzSlotChars), "eu-west-1")

	var auto bytes.Buffer

	fmt.Fprintf(&auto, "locals {\n  region = %q\n", regionLocal)

	if c.boolean() {
		fmt.Fprintf(&auto, "  tier = %q\n", c.choose([]string{"prod", "staging", "dev"}))
	}

	if c.boolean() {
		fmt.Fprintf(&auto, "  enabled = %t\n", c.boolean())
	}

	auto.WriteString("}\n")

	var main bytes.Buffer

	main.WriteString("terraform {\n  source = \"./mod\"\n}\n\ninputs = {\n  region = \"us-east-1\"\n")

	if c.boolean() {
		main.WriteString("  shared = \"value\"\n")
	}

	main.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.autoinclude.hcl", data: auto.Bytes()},
		{path: fuzzWorkDir + "/terragrunt.hcl", data: main.Bytes()},
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
// are bounded by the fuzz HTTP handler rather than real network. Backend
// type, disable flags, generate.if_exists, and the per-backend config
// alphabet each flip from the fuzz stream.
func fuzzShapeRemoteState(c *consumer) []fuzzSeedFile {
	backend := c.choose([]string{"s3", "gcs", "local"})
	bucket := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz-bucket")
	key := orDefault(c.slot(fuzzMaxFuzzSlotChars), "tfstate")
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")

	var b bytes.Buffer

	fmt.Fprintf(&b, "remote_state {\n  backend = %q\n", backend)

	if c.boolean() {
		fmt.Fprintf(&b, "  disable_init = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  disable_dependency_optimization = %t\n", c.boolean())
	}

	b.WriteString("  config = {\n")
	fmt.Fprintf(&b, "    bucket = %q\n    key    = %q\n    region = %q\n", bucket, key, region)

	if c.boolean() {
		fmt.Fprintf(&b, "    encrypt = %t\n", c.boolean())
	}

	if backend == "s3" && c.boolean() {
		fmt.Fprintf(&b, "    dynamodb_table = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz-lock"))
	}

	if backend == "s3" && c.boolean() {
		fmt.Fprintf(&b, "    role_arn = %q\n", "arn:aws:iam::123456789012:role/"+orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz"))
	}

	if backend == "gcs" && c.boolean() {
		fmt.Fprintf(&b, "    location = %q\n", c.choose([]string{"US", "EU", "ASIA"}))
	}

	if backend == "gcs" && c.boolean() {
		fmt.Fprintf(&b, "    project = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz-project"))
	}

	b.WriteString("  }\n")

	if c.boolean() {
		fmt.Fprintf(&b, "  generate = {\n    path      = \"backend.tf\"\n    if_exists = %q\n  }\n",
			c.choose([]string{"overwrite", "overwrite_terragrunt", "skip"}))
	}

	b.WriteString(`}

terraform {
  source = "./mod"
}

inputs = {
`)
	fmt.Fprintf(&b, "  region = %q\n", region)

	if c.boolean() {
		fmt.Fprintf(&b, "  bucket = %q\n", bucket)
	}

	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeGenerate exercises codegen's contents/signature/if_exists/comment
// preparation. The final os.WriteFile under /work fails fast on macOS/CI
// (parent missing), so only the pre-write logic is covered. Block count,
// per-block contents, if_exists, comment_prefix, disable_signature, and
// hcl_fmt each flip from the fuzz stream.
func fuzzShapeGenerate(c *consumer) []fuzzSeedFile {
	name := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "provider")
	contents := orDefault(c.slot(fuzzMaxFuzzSlotChars*4), `provider "null" {}`)
	ifExists := c.choose([]string{"overwrite", "overwrite_terragrunt", "skip"})
	commentPrefix := orDefault(c.slot(fuzzMaxFuzzSlotChars), "# ")

	var b bytes.Buffer

	fmt.Fprintf(&b, "generate %q {\n  path     = \"%s.tf\"\n  contents = %q\n  if_exists = %q\n",
		name, name, contents, ifExists)

	if c.boolean() {
		fmt.Fprintf(&b, "  comment_prefix = %q\n", commentPrefix)
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  disable_signature = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  hcl_fmt = %t\n", c.boolean())
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  disable = %t\n", c.boolean())
	}

	b.WriteString("}\n\n")

	if c.boolean() {
		fmt.Fprintf(&b, "generate \"backend\" {\n  path      = \"backend.tf\"\n  contents  = \"terraform { backend \\\"local\\\" {} }\"\n  if_exists = %q\n}\n\n",
			c.choose([]string{"overwrite", "overwrite_terragrunt", "skip"}))
	}

	if c.boolean() {
		fmt.Fprintf(&b, "generate \"versions\" {\n  path      = \"versions.tf\"\n  contents  = \"terraform { required_version = %q }\"\n  if_exists = \"overwrite\"\n}\n\n",
			c.choose([]string{">= 1.0", "~> 1.5"}))
	}

	b.WriteString("terraform {\n  source = \"./mod\"\n}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeFeatureFlags drives feature-flag parse and substitution in
// inputs evaluation. The default value's HCL type flips between string,
// number, bool, and list; a second feature block optionally appears.
func fuzzShapeFeatureFlags(c *consumer) []fuzzSeedFile {
	// feat is emitted both as a quoted block label and as an unquoted
	// traversal step (`feature.<feat>.value`). slotIdent ensures the
	// unquoted form lexes as an HCL identifier rather than a numeric
	// literal, which would otherwise allow `1e9999999`-style precision
	// bombs through cty.
	feat := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "feat")

	defVal := func() string {
		switch c.intN(5) {
		case 0:
			return fmt.Sprintf("%q", orDefault(c.slot(fuzzMaxFuzzSlotChars), "default"))
		case 1:
			return strconv.Itoa(c.intN(1000))
		case 2:
			if c.boolean() {
				return "true"
			}

			return "false"
		case 3:
			return `["a", "b", "c"]`
		default:
			return `{ k = "v" }`
		}
	}()

	var b bytes.Buffer

	fmt.Fprintf(&b, "feature %q {\n  default = %s\n}\n\n", feat, defVal)

	if c.boolean() {
		feat2 := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "feat2")
		fmt.Fprintf(&b, "feature %q {\n  default = true\n}\n\n", feat2)
	}

	b.WriteString("terraform {\n  source = \"./mod\"\n}\n\ninputs = {\n")
	fmt.Fprintf(&b, "  x = feature.%s.value\n", feat)
	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeHooks routes before/after/error hook execution through v.Exec
// via shell.RunCommandWithOutput. extra_arguments exercises arg-injection.
// Each hook flavour is independently flipped, and within each hook
// run_on_error / working_dir / suppress_stdout each toggle a parse path.
func fuzzShapeHooks(c *consumer) []fuzzSeedFile {
	cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
	arg := orDefault(c.slot(fuzzMaxFuzzSlotChars), "fuzz")

	var b bytes.Buffer

	b.WriteString("terraform {\n  source = \"./mod\"\n\n")

	if c.boolean() {
		fmt.Fprintf(&b, "  before_hook \"pre\" {\n    commands = [%q]\n    execute  = [\"echo\", %q]\n", cmd, arg)

		if c.boolean() {
			fmt.Fprintf(&b, "    working_dir = %q\n", fuzzWorkDir)
		}

		if c.boolean() {
			fmt.Fprintf(&b, "    suppress_stdout = %t\n", c.boolean())
		}

		b.WriteString("  }\n\n")
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  after_hook \"post\" {\n    commands     = [%q]\n    execute      = [\"echo\", \"after\"]\n    run_on_error = %t\n",
			cmd, c.boolean())

		if c.boolean() {
			fmt.Fprintf(&b, "    working_dir = %q\n", fuzzWorkDir)
		}

		b.WriteString("  }\n\n")
	}

	if c.boolean() {
		pat := c.choose([]string{".*", ".*timeout.*", ".*lock.*", "ERROR.*"})
		fmt.Fprintf(&b, "  error_hook \"err\" {\n    commands  = [%q]\n    on_errors = [%q]\n    execute   = [\"echo\", \"err\"]\n",
			cmd, pat)

		if c.boolean() {
			fmt.Fprintf(&b, "    run_on_error = %t\n", c.boolean())
		}

		b.WriteString("  }\n\n")
	}

	if c.boolean() {
		fmt.Fprintf(&b, "  extra_arguments \"extra\" {\n    commands  = [%q]\n    arguments = [\"-var\", \"x=1\"]\n    env_vars  = {\n      FUZZ = %q\n    }\n",
			cmd, arg)

		if c.boolean() {
			b.WriteString("    required_var_files = []\n")
		}

		if c.boolean() {
			b.WriteString("    optional_var_files = []\n")
		}

		b.WriteString("  }\n")
	}

	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
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

// fuzzShapeBuiltinFuncs drives the long tail of Terragrunt's HCL helper
// functions registered in pkg/config/config_helpers.go. Most of these are
// pure (no disk, no network, no AWS metadata) or fail cheaply when their
// arguments are nonsense, so they're safe to exercise with fuzzed slot
// strings. The shape leans on `try(...)` so a function whose runtime
// arguments don't validate (e.g. timecmp on a malformed timestamp)
// still produces a parseable config — the goal is exercising HCL eval
// branches, not asserting on results.
func fuzzShapeBuiltinFuncs(c *consumer) []fuzzSeedFile {
	a := orDefault(c.slot(fuzzMaxFuzzSlotChars), "a")
	b := orDefault(c.slot(fuzzMaxFuzzSlotChars), "b")
	env := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "HOME")
	ts1 := orDefault(c.slot(fuzzMaxFuzzSlotChars), "2025-01-01T00:00:00Z")
	ts2 := orDefault(c.slot(fuzzMaxFuzzSlotChars), "2025-06-01T00:00:00Z")
	constraint := orDefault(c.slot(fuzzMaxFuzzSlotChars), ">= 1.0")
	version := orDefault(c.slot(fuzzMaxFuzzSlotChars), "1.5.0")
	glob := orDefault(c.slot(fuzzMaxFuzzSlotChars), "*.hcl")
	hcl := fmt.Sprintf(`
locals {
  platform     = get_platform()
  workdir      = get_working_dir()
  tg_dir       = get_terragrunt_dir()
  tg_command   = get_terraform_command()
  tg_cli       = get_terraform_cli_args()
  need_vars    = get_terraform_commands_that_need_vars()
  need_locking = get_terraform_commands_that_need_locking()
  src_flag     = get_terragrunt_source_cli_flag()
  retry_errs   = get_default_retryable_errors()

  env_val    = try(get_env(%q, "default"), "fallback")
  starts     = startswith(%q, %q)
  ends       = endswith(%q, %q)
  contains   = strcontains(%q, %q)
  time_cmp   = try(timecmp(%q, %q), 0)
  constraint = try(constraint_check(%q, %q), false)
}

terraform {
  source = "./mod"
}

inputs = {
  platform     = local.platform
  workdir      = local.workdir
  tg_dir       = local.tg_dir
  tg_command   = local.tg_command
  tg_cli       = local.tg_cli
  need_vars    = local.need_vars
  need_locking = local.need_locking
  src_flag     = local.src_flag
  retry_errs   = local.retry_errs
  env_val      = local.env_val
  starts       = local.starts
  ends         = local.ends
  contains     = local.contains
  time_cmp     = local.time_cmp
  constraint   = local.constraint
  marked       = try(mark_as_read(%q), "")
  marked_glob  = try(mark_glob_as_read(%q), "")
}
`, env, a, b, a, b, a, b, ts1, ts2, version, constraint, a, glob)

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: []byte(hcl)},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeMultiUnitRunAll lays down a small DAG of units so a `run --all`
// invocation drives the queue construction, dependency resolution, and
// unit-runner scheduling code paths. The base diamond (B and C depend on
// A) is always present; the fuzz stream optionally adds a depth-2 unit D
// (depends on B and C) and a disconnected unit E. Each unit's skip flag
// and mock-output attributes flip independently.
func fuzzShapeMultiUnitRunAll(c *consumer) []fuzzSeedFile {
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")
	envName := orDefault(c.slot(fuzzMaxFuzzSlotChars), "dev")

	root := fmt.Sprintf("locals {\n  region = %q\n  env    = %q\n}\n", region, envName)

	buildUnit := func(name string, deps []string) []byte {
		var b bytes.Buffer

		b.WriteString("include \"root\" {\n  path = find_in_parent_folders(\"root.hcl\")\n}\n\n")

		for _, d := range deps {
			fmt.Fprintf(&b, "dependency %q {\n  config_path = \"../%s\"\n", d, d)

			if c.boolean() {
				fmt.Fprintf(&b, "  mock_outputs = {\n    id = \"mock-%s-id\"\n  }\n", d)
			}

			if c.boolean() {
				b.WriteString("  mock_outputs_allowed_terraform_commands = [\"plan\", \"apply\", \"destroy\", \"validate\", \"init\"]\n")
			}

			if c.boolean() {
				fmt.Fprintf(&b, "  mock_outputs_merge_strategy_with_state = %q\n", c.choose([]string{"no_merge", "shallow", "deep"}))
			}

			if c.boolean() {
				fmt.Fprintf(&b, "  skip_outputs = %t\n", c.boolean())
			}

			b.WriteString("}\n\n")
		}

		if c.boolean() {
			fmt.Fprintf(&b, "skip = %t\n\n", c.boolean())
		}

		b.WriteString("terraform {\n  source = \"../mod\"\n}\n\ninputs = {\n")
		fmt.Fprintf(&b, "  name   = %q\n  region = include.root.locals.region\n", name)

		for _, d := range deps {
			fmt.Fprintf(&b, "  %s_id = dependency.%s.outputs.id\n", d, d)
		}

		b.WriteString("}\n")

		return b.Bytes()
	}

	files := []fuzzSeedFile{
		{path: fuzzWorkDir + "/root.hcl", data: []byte(root)},
		{path: fuzzWorkDir + "/a/terragrunt.hcl", data: buildUnit("a", nil)},
		{path: fuzzWorkDir + "/b/terragrunt.hcl", data: buildUnit("b", []string{"a"})},
		{path: fuzzWorkDir + "/c/terragrunt.hcl", data: buildUnit("c", []string{"a"})},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}
output "id" { value = "stub-id" }`)},
	}

	if c.boolean() {
		files = append(files, fuzzSeedFile{
			path: fuzzWorkDir + "/d/terragrunt.hcl",
			data: buildUnit("d", []string{"b", "c"}),
		})
	}

	if c.boolean() {
		files = append(files, fuzzSeedFile{
			path: fuzzWorkDir + "/e/terragrunt.hcl",
			data: buildUnit("e", nil),
		})
	}

	return files
}

// fuzzShapeExclude exercises the exclude block evaluation paths. The
// `if` condition, action set, and dependency-exclusion bit each flip
// from the fuzz stream.
func fuzzShapeExclude(c *consumer) []fuzzSeedFile {
	action := c.choose([]string{"apply", "plan", "destroy", "all"})
	noRun := c.boolean()
	excludeDeps := c.boolean()

	condition := "true"
	if c.boolean() {
		condition = "false"
	}

	var b bytes.Buffer

	fmt.Fprintf(&b, "exclude {\n  if      = %s\n  actions = [%q",
		condition, action)

	if c.boolean() {
		fmt.Fprintf(&b, ", %q", c.choose([]string{"apply", "plan", "destroy", "init", "validate"}))
	}

	fmt.Fprintf(&b, "]\n  no_run               = %t\n  exclude_dependencies = %t\n", noRun, excludeDeps)
	b.WriteString("}\n\nterraform {\n  source = \"./mod\"\n}\n")

	if c.boolean() {
		b.WriteString("\ninputs = {\n  name = \"fuzz\"\n}\n")
	}

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeErrorsIgnore exercises errors { ignore "x" { ... } } alongside
// the retry variant covered by fuzzShapeErrorsBlock. The signals map size,
// pattern list, and the presence of a second ignore block each flip.
func fuzzShapeErrorsIgnore(c *consumer) []fuzzSeedFile {
	pattern := orDefault(c.slot(fuzzMaxFuzzSlotChars), "ignored")
	sigVal := orDefault(c.slot(fuzzMaxFuzzSlotChars), "sigval")
	msg := orDefault(c.slot(fuzzMaxFuzzSlotChars), "ignored error")

	var b bytes.Buffer

	b.WriteString("errors {\n")
	b.WriteString("  retry \"transient\" {\n    retryable_errors   = [\".*timeout.*\"]\n    max_attempts       = 2\n    sleep_interval_sec = 1\n  }\n")
	fmt.Fprintf(&b, "  ignore \"known\" {\n    ignorable_errors = [%q", pattern)

	if c.boolean() {
		fmt.Fprintf(&b, ", %q", c.choose([]string{".*lock.*", ".*deadline.*", ".*conflict.*"}))
	}

	fmt.Fprintf(&b, "]\n    message          = %q\n", msg)
	b.WriteString("    signals = {\n")
	fmt.Fprintf(&b, "      foo = %q\n", sigVal)

	if c.boolean() {
		fmt.Fprintf(&b, "      bar = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "barval"))
	}

	b.WriteString("    }\n  }\n")

	if c.boolean() {
		fmt.Fprintf(&b, "  ignore \"second\" {\n    ignorable_errors = [%q]\n    message          = \"second\"\n  }\n",
			c.choose([]string{".*5xx.*", ".*throttl.*", ".*temp.*"}))
	}

	b.WriteString("}\n\nterraform {\n  source = \"./mod\"\n}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeExtraArgs exercises the extra_arguments arg-injection path.
// Command set size, env_vars population, second arg block, and the
// optional/required var file lists each flip from the fuzz stream.
func fuzzShapeExtraArgs(c *consumer) []fuzzSeedFile {
	cmd := c.choose([]string{"apply", "plan", "destroy", "init"})
	val := orDefault(c.slot(fuzzMaxFuzzSlotChars), "v")

	var b bytes.Buffer

	b.WriteString("terraform {\n  source = \"./mod\"\n\n")

	fmt.Fprintf(&b, "  extra_arguments \"vars\" {\n    commands  = [%q", cmd)

	if c.boolean() {
		fmt.Fprintf(&b, ", %q", c.choose([]string{"apply", "plan", "destroy", "init", "validate"}))
	}

	fmt.Fprintf(&b, "]\n    arguments = [\"-var\", \"x=%s\"]\n", val)

	if c.boolean() {
		fmt.Fprintf(&b, "    env_vars = {\n      FUZZ = %q\n", val)

		if c.boolean() {
			fmt.Fprintf(&b, "      BAR  = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "barval"))
		}

		b.WriteString("    }\n")
	}

	if c.boolean() {
		b.WriteString("    required_var_files = []\n")
	}

	if c.boolean() {
		b.WriteString("    optional_var_files = []\n")
	}

	b.WriteString("  }\n")

	if c.boolean() {
		fmt.Fprintf(&b, "\n  extra_arguments \"defaults\" {\n    commands  = [%q]\n    arguments = [\"-input=false\"]\n  }\n",
			c.choose([]string{"apply", "plan", "destroy", "init"}))
	}

	b.WriteString("}\n")

	return []fuzzSeedFile{
		{path: fuzzWorkDir + "/terragrunt.hcl", data: b.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}
}

// fuzzShapeStackNested exercises stack-level locals, include, values, and
// nested stack-of-stacks composition. The values map content, optional
// nested stack-of-stacks, and per-unit no_dot_terragrunt_stack each flip.
func fuzzShapeStackNested(c *consumer) []fuzzSeedFile {
	region := orDefault(c.slot(fuzzMaxFuzzSlotChars), "us-east-1")
	childName := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "child")

	var stack bytes.Buffer

	fmt.Fprintf(&stack, "locals {\n  region = %q\n", region)

	if c.boolean() {
		fmt.Fprintf(&stack, "  env = %q\n", c.choose([]string{"dev", "staging", "prod"}))
	}

	stack.WriteString("}\n\nunit \"a\" {\n  source = \"./units/a\"\n  path   = \"live/a\"\n  values = {\n    region = local.region\n")

	if c.boolean() {
		fmt.Fprintf(&stack, "    name   = %q\n", orDefault(c.slot(fuzzMaxFuzzSlotChars), "a"))
	}

	stack.WriteString("  }\n")

	if c.boolean() {
		fmt.Fprintf(&stack, "  no_dot_terragrunt_stack = %t\n", c.boolean())
	}

	stack.WriteString("}\n\n")
	fmt.Fprintf(&stack, "stack %q {\n  source = \"./stacks/child\"\n  path   = \"child\"\n}\n", childName)

	if c.boolean() {
		fmt.Fprintf(&stack, "\nstack \"sibling\" {\n  source = \"./stacks/%s\"\n  path   = \"sibling\"\n}\n", childName)
	}

	stackHCL := stack.String()

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
// The path count and the presence of a third sibling unit each flip.
func fuzzShapeDependenciesPaths(c *consumer) []fuzzSeedFile {
	extra := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "other")

	var app bytes.Buffer

	app.WriteString("dependencies {\n  paths = [\"../db\"")

	if c.boolean() {
		fmt.Fprintf(&app, ", \"../%s\"", extra)
	}

	third := orDefault(c.slotIdent(fuzzMaxFuzzSlotChars), "third")
	includeThird := c.boolean()

	if includeThird {
		fmt.Fprintf(&app, ", \"../%s\"", third)
	}

	app.WriteString("]\n}\n\nterraform {\n  source = \"../mod\"\n}\n")

	files := []fuzzSeedFile{
		{path: fuzzWorkDir + "/db/terragrunt.hcl", data: []byte("terraform {\n  source = \"../mod\"\n}\n")},
		{path: fuzzWorkDir + "/" + extra + "/terragrunt.hcl", data: []byte("terraform {\n  source = \"../mod\"\n}\n")},
		{path: fuzzWorkDir + "/app/terragrunt.hcl", data: app.Bytes()},
		{path: fuzzWorkDir + "/mod/main.tf", data: []byte(`resource "null_resource" "x" {}`)},
	}

	if includeThird {
		files = append(files, fuzzSeedFile{
			path: fuzzWorkDir + "/" + third + "/terragrunt.hcl",
			data: []byte("terraform {\n  source = \"../mod\"\n}\n"),
		})
	}

	return files
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
//
// For tofu/terraform subcommands whose stdout Terragrunt parses (output,
// show, state), the handler picks from a curated pool of realistic JSON
// shapes alongside the random slot. Random alone almost never satisfies
// the downstream JSON/cty decoders, so dependency.outputs evaluation,
// plan summarization, and similar post-parse code paths stay unreachable.
// Letting the fuzz stream choose (valid, malformed, random) widens the
// acceptance funnel without losing the random-bytes coverage.
func fuzzExecHandler(c *consumer) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if inv.Name == "tofu" || inv.Name == "terraform" {
			if r, ok := fuzzTFResult(c, &inv); ok {
				return r
			}
		}

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

// fuzzTFResult returns a curated response for tofu/terraform subcommands
// the codebase parses. The second return is false when the subcommand
// has no special handling, so the caller falls through to the random
// stdout/exit path.
func fuzzTFResult(c *consumer, inv *vexec.Invocation) (vexec.Result, bool) {
	sub := tfSubcommand(inv.Args)
	switch sub {
	case "-version", "--version", "version":
		return vexec.Result{Stdout: []byte("OpenTofu v1.8.0\non darwin_arm64\n")}, true
	case "output":
		return vexec.Result{Stdout: fuzzTFOutputStdout(c)}, true
	case "show":
		return vexec.Result{Stdout: fuzzTFShowStdout(c)}, true
	case "state":
		return vexec.Result{Stdout: fuzzTFStateStdout(c)}, true
	}

	return vexec.Result{}, false
}

// tfSubcommand returns the first non-flag argument from a tofu/terraform
// invocation, skipping leading globals like `-chdir=/work` that Terragrunt
// prepends.
func tfSubcommand(args []string) string {
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}

		return a
	}

	return ""
}

// tofu output -json shapes: empty, single string, nested object, malformed,
// or random. Exit code is 0 so the parse path is exercised.
func fuzzTFOutputStdout(c *consumer) []byte {
	switch c.byteOnce() & 3 {
	case 0:
		return []byte(`{}`)
	case 1:
		return []byte(`{"id":{"value":"x","type":"string","sensitive":false}}`)
	case 2:
		return []byte(`{"out":{"value":{"k":"v"},"type":["object",{"k":"string"}],"sensitive":false}}`)
	default:
		return []byte(c.slot(fuzzMaxExecOutLen))
	}
}

// tofu show -json shapes: minimal plan, plan with a single resource change,
// malformed, or random.
func fuzzTFShowStdout(c *consumer) []byte {
	switch c.byteOnce() & 3 {
	case 0:
		return []byte(`{"format_version":"1.2","terraform_version":"1.6.0","planned_values":{"root_module":{}},"resource_changes":[],"output_changes":{},"configuration":{"root_module":{}}}`)
	case 1:
		return []byte(`{"format_version":"1.2"}`)
	case 2:
		return []byte(`{"format_version":"1.2","resource_changes":[{"address":"null_resource.x","mode":"managed","type":"null_resource","name":"x","change":{"actions":["create"],"before":null,"after":{}}}]}`)
	default:
		return []byte(c.slot(fuzzMaxExecOutLen))
	}
}

// tofu state pull / state show shapes: empty state, populated state, or random.
func fuzzTFStateStdout(c *consumer) []byte {
	switch c.byteOnce() & 3 {
	case 0:
		return []byte(`{"version":4,"terraform_version":"1.6.0","serial":1,"lineage":"abc","outputs":{},"resources":[]}`)
	case 1:
		return []byte(`{"version":4,"outputs":{"id":{"value":"x","type":"string"}},"resources":[]}`)
	default:
		return []byte(c.slot(fuzzMaxExecOutLen))
	}
}

// fuzzHTTPHandler synthesizes responses for every outbound HTTP request.
// AWS and GCP SDKs route through the same handler via vhttp.
//
// For STS, S3, IMDS, and GCS the handler picks from a small pool of
// realistic XML/JSON envelopes alongside the random slot. Bare random
// bytes never decode as AWS SDK shapes, so credential propagation, S3
// bucket-validation, and downstream backend builders stay unreachable.
// Returning a real-shaped AssumeRoleResponse occasionally lets the SDK
// pass auth and exercise the next layer.
func fuzzHTTPHandler(c *consumer) vhttp.Handler {
	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		if r, ok := fuzzAWSResponse(c, req); ok {
			return r, nil
		}

		if r, ok := fuzzGCSResponse(c, req); ok {
			return r, nil
		}

		status := 200 + int(c.byteOnce()%3)*100

		body := []byte("{}")
		if c.boolean() {
			body = []byte(c.slot(fuzzMaxHTTPBodyLen))
		}

		return vhttp.Respond(status, body, nil), nil
	}
}

// fuzzAWSResponse routes requests bound for AWS service endpoints to a
// curated XML response pool. Hosts are matched on the conventional SDK
// patterns; anything else returns ok=false and falls through.
func fuzzAWSResponse(c *consumer, req *http.Request) (*http.Response, bool) {
	host := req.URL.Host

	switch {
	case strings.HasPrefix(host, "sts."):
		return fuzzSTSResponse(c, req), true
	case strings.HasPrefix(host, "s3.") || strings.Contains(host, ".s3."):
		return fuzzS3Response(c), true
	case host == "169.254.169.254":
		// IMDS — return 404 so the SDK gives up on instance-profile auth
		// instead of retrying. Real-shaped credentials would mislead the
		// rest of the chain into a stable AWS identity for the whole run.
		return vhttp.Respond(http.StatusNotFound, nil, nil), true
	}

	return nil, false
}

const (
	stsAssumeRoleResponseXML = `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleResult>
    <Credentials>
      <SessionToken>FQoG-fuzz-session-token</SessionToken>
      <SecretAccessKey>fuzz-secret-access-key</SecretAccessKey>
      <Expiration>2099-01-01T00:00:00Z</Expiration>
      <AccessKeyId>AKIAFUZZFUZZFUZZFUZZ</AccessKeyId>
    </Credentials>
    <AssumedRoleUser>
      <Arn>arn:aws:sts::123456789012:assumed-role/r/fuzz</Arn>
      <AssumedRoleId>AROAFUZZ:fuzz</AssumedRoleId>
    </AssumedRoleUser>
  </AssumeRoleResult>
  <ResponseMetadata>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
  </ResponseMetadata>
</AssumeRoleResponse>`

	stsGetCallerIdentityResponseXML = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/fuzz</Arn>
    <UserId>AIDAFUZZ:fuzz</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`

	stsAccessDeniedXML = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error>
    <Type>Sender</Type>
    <Code>AccessDenied</Code>
    <Message>User is not authorized to perform sts:AssumeRole</Message>
  </Error>
  <RequestId>00000000-0000-0000-0000-000000000000</RequestId>
</ErrorResponse>`

	s3NoSuchBucketXML = `<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>NoSuchBucket</Code>
  <Message>The specified bucket does not exist</Message>
  <BucketName>fuzz-bucket</BucketName>
  <RequestId>00000000000000000000</RequestId>
</Error>`
)

// fuzzSTSResponse picks a realistic STS XML response shape based on the
// Action= field in the request body. AWS SDK v2 POSTs form-encoded params
// with Action determining the response shape the decoder expects.
func fuzzSTSResponse(c *consumer, req *http.Request) *http.Response {
	headers := http.Header{}
	headers.Set("Content-Type", "text/xml")

	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
	}

	switch {
	case bytes.Contains(body, []byte("AssumeRole")):
		switch c.byteOnce() & 3 {
		case 0, 1:
			return vhttp.Respond(http.StatusOK, []byte(stsAssumeRoleResponseXML), headers)
		case 2:
			return vhttp.Respond(http.StatusForbidden, []byte(stsAccessDeniedXML), headers)
		default:
			return vhttp.Respond(http.StatusOK, []byte(c.slot(fuzzMaxHTTPBodyLen)), headers)
		}
	case bytes.Contains(body, []byte("GetCallerIdentity")):
		if c.byteOnce()&1 == 0 {
			return vhttp.Respond(http.StatusOK, []byte(stsGetCallerIdentityResponseXML), headers)
		}

		return vhttp.Respond(http.StatusOK, []byte(c.slot(fuzzMaxHTTPBodyLen)), headers)
	}

	return vhttp.Respond(http.StatusOK, []byte(c.slot(fuzzMaxHTTPBodyLen)), headers)
}

// fuzzS3Response picks among 200 OK with empty body (head-bucket happy
// path), a NoSuchBucket XML error, and random bytes.
func fuzzS3Response(c *consumer) *http.Response {
	switch c.byteOnce() & 3 {
	case 0, 1:
		return vhttp.Respond(http.StatusOK, nil, nil)
	case 2:
		headers := http.Header{}
		headers.Set("Content-Type", "application/xml")

		return vhttp.Respond(http.StatusNotFound, []byte(s3NoSuchBucketXML), headers)
	default:
		return vhttp.Respond(http.StatusOK, []byte(c.slot(fuzzMaxHTTPBodyLen)), nil)
	}
}

// fuzzGCSResponse routes requests bound for GCS service endpoints to a
// curated JSON response pool.
func fuzzGCSResponse(c *consumer, req *http.Request) (*http.Response, bool) {
	host := req.URL.Host
	if !strings.HasSuffix(host, ".googleapis.com") && host != "googleapis.com" {
		return nil, false
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	switch c.byteOnce() & 3 {
	case 0, 1:
		return vhttp.Respond(http.StatusOK, []byte(`{"kind":"storage#bucket","name":"fuzz-bucket","location":"US"}`), headers), true
	case 2:
		return vhttp.Respond(http.StatusNotFound, []byte(`{"error":{"code":404,"message":"Not Found"}}`), headers), true
	default:
		return vhttp.Respond(http.StatusOK, []byte(c.slot(fuzzMaxHTTPBodyLen)), headers), true
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

		// cli.App.RunContext expects args[0] to be the program name (the
		// usual os.Args convention). Without it, args[0] is consumed as the
		// program identifier and the actual command shifts off the end, so
		// every dispatch silently falls back to the help text.
		args := slices.Concat([]string{"terragrunt"}, w.args)

		// Watchdog so iterations that ignore context cancellation surface
		// as a finding the fuzz framework can record. Without this, a hot
		// path that doesn't check ctx (e.g. cty/big.Float ↔ string
		// conversion on huge precision) blocks RunContext past the framework's
		// worker timeout, which then reports an opaque "EOF" instead of a
		// minimizable failure.
		//
		// Elapsed is measured inside the worker goroutine so the slow-iteration
		// check below reflects RunContext's actual wall-clock, not the
		// scheduling latency between RunContext returning and main getting
		// scheduled to read the channel. Under fully-saturated parallel fuzz
		// workers, that scheduler jitter can add hundreds of milliseconds and
		// produce minimized "findings" that don't reproduce.
		type runResult struct {
			err     error
			elapsed time.Duration
		}

		done := make(chan runResult, 1)

		go func() {
			s := time.Now()

			e := app.RunContext(ctx, args)
			done <- runResult{err: e, elapsed: time.Since(s)}
		}()

		var (
			err     error
			elapsed time.Duration
		)

		select {
		case r := <-done:
			err, elapsed = r.err, r.elapsed
		case <-time.After(fuzzPerRunTimeout + time.Second):
			t.Fatalf("iteration hung past %s without honoring context cancellation; args=%q", fuzzPerRunTimeout+time.Second, args)
		}

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
