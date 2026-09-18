package test_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/require"
)

const (
	// fuzzPerRunTimeout caps one iteration, so a retry loop or a sleep in an
	// errors block cannot stall a fuzz worker.
	fuzzPerRunTimeout = 5 * time.Second

	fuzzFixturesDir     = "fixtures"
	fuzzMaxFixtureFile  = 64 << 10
	fuzzMaxFlags        = 5
	fuzzMaxPositionals  = 2
	fuzzMaxTFArgs       = 3
	fuzzMaxEnvEntries   = 6
	fuzzMaxSlot         = 24
	fuzzMaxMalformedHCL = 256
	fuzzMaxSubprocOut   = 96
	fuzzMaxHTTPBody     = 128
	fuzzFileMode        = 0o644
)

// EnvFuzzSkipList names a file of failure signatures, one per line, that
// [FuzzFullCLI] skips instead of failing on, so a long fuzz run gets past a
// bug it already reported.
const EnvFuzzSkipList = "TG_TEST_FUZZ_SKIP_LIST"

// fuzzModuleFramePrefix marks the stack frames a signature is taken from.
const fuzzModuleFramePrefix = "github.com/gruntwork-io/terragrunt/"

var (
	fuzzDigitsRe      = regexp.MustCompile(`[0-9]+`)
	fuzzFrameArgsRe   = regexp.MustCompile(`\([^()]*\)$`)
	fuzzMessageDataRe = regexp.MustCompile(`[("'].*$`)
)

// fuzzRoot is where every iteration's world is laid down in memory, and the
// directory the CLI starts in.
var fuzzRoot = venvtest.Root("/work")

// fuzzExcludedCommands are command paths the fuzz never invokes.
//
//   - browse and catalog start a Bubble Tea program that takes over the
//     process's terminal, which no venv handle intercepts.
var fuzzExcludedCommands = map[string]struct{}{
	"browse":  {},
	"catalog": {},
}

// FuzzFullCLI drives the whole Terragrunt CLI, from argument parsing down to
// the subprocess and HTTP calls a command makes, through an in-memory venv.
// Each input deterministically derives one invocation:
//
//  1. A command path, picked from the live command tree of [cli.NewApp], so
//     every command and subcommand is reachable without this file listing
//     them.
//  2. A world: either a synthetic layout aimed at one part of the config
//     pipeline, or one of the real trees under test/fixtures. One file in it
//     may be mutated, so the parser also sees broken input.
//  3. Flags drawn from those the command accepts (its own, its ancestors',
//     and the global ones), with values from a shared pool or the input.
//  4. Environment variables drawn from the env vars those flags read, plus
//     cloud and OpenTofu variables.
//  5. Answers for every subprocess and HTTP request, either a plausible reply
//     so the command gets further, or bytes taken from the input.
//
// An iteration fails on a panic, including one the CLI recovers and reports
// as a crash, or on a run that exits with an error without writing anything
// to stderr while logging is enabled. Each failure logs a line starting with
// "fuzz signature: ". A failure whose signature is listed in the file
// [EnvFuzzSkipList] names is skipped instead. A panic on a goroutine the CLI
// spawns ends the worker process before the test sees it, so it cannot be
// skipped.
//
// Subprocess and HTTP handlers draw from the same input as the rest, so when
// run --all fans out, the order they draw in varies between runs. A
// reproducer for such a failure may need a few runs to trip again.
func FuzzFullCLI(f *testing.F) {
	vocab := newFuzzVocab(f)
	worlds := newFuzzWorlds(f)
	skips := loadFuzzSkipList(f)

	// One seed per command path, each paired with a different world, so plain
	// `go test` runs every command at least once.
	for i := range vocab.commands {
		seed := binary.LittleEndian.AppendUint16(nil, uint16(i))
		seed = binary.LittleEndian.AppendUint16(seed, uint16(i%len(worlds)))
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		c := newFuzzConsumer(data)
		inv := vocab.invocation(c, worlds)

		defer func() {
			if r := recover(); r != nil {
				sig := fuzzPanicSignature(fmt.Sprint(r), debug.Stack())
				failFuzz(t, skips, sig, "panic: %v\n%s\nargs=%q\nenv=%q", r, debug.Stack(), inv.args, inv.env)
			}
		}()

		fsys := vfs.NewMemMapFS()
		for path, contents := range inv.files {
			require.NoError(t, vfs.WriteFile(fsys, path, contents, fuzzFileMode))
		}

		stdout, stderr := &fuzzCountingWriter{}, &fuzzCountingWriter{}

		v := venvtest.New().
			WithFS(fsys).
			WithExec(vexec.NewMemExec(fuzzExecHandler(c), vexec.WithLookPath(fuzzLookPath))).
			WithHTTP(vhttp.NewMemClient(fuzzHTTPHandler(c))).
			WithEnv(inv.env).
			WithStdin(strings.NewReader(inv.stdin)).
			WithWriter(stdout).
			WithErrWriter(stderr)

		platform := *v.Platform
		platform.Getwd = func() (string, error) { return fuzzRoot, nil }
		v.Platform = &platform

		l := logger.CreateLogger().WithOptions(
			log.WithOutput(v.Writers.ErrWriter),
			log.WithLevel(options.DefaultLogLevel),
		)

		opts := options.NewTerragruntOptions(v.Exec)
		app := cli.NewApp(l, opts, v)
		em := tf.NewDetailedExitCodeMap()

		ctx, cancel := context.WithTimeout(t.Context(), fuzzPerRunTimeout)
		defer cancel()

		ctx = log.ContextWithLogger(ctx, l)
		ctx = tf.ContextWithDetailedExitCode(ctx, em)

		args := append([]string{cli.AppName}, inv.args...)

		err := app.RunContext(ctx, l, v, args)

		if panicreport.IsPanic(err) {
			msg, stack := panicreport.PanicDetails(err)
			failFuzz(t, skips, fuzzPanicSignature(msg, stack),
				"recovered panic: %s\n%s\nargs=%q\nenv=%q", msg, stack, inv.args, inv.env)
		}

		// ExitCodeFor is what main.go runs after RunContext; it logs the error.
		cli.ExitCodeFor(l, args, app.Version, err, 0, panicreport.New(v))

		silenced := l.Formatter().DisabledOutput() || l.Level() < log.ErrorLevel
		silentFailure := err != nil && stderr.n.Load() == 0 && ctx.Err() == nil && !silenced &&
			!errors.Is(err, runall.ErrUserCancelled)
		if silentFailure {
			sig := fmt.Sprintf("silent failure: %s %T", strings.Join(inv.command, " "), err)
			failFuzz(t, skips, sig, "exited with %v but wrote nothing to stderr\nargs=%q\nenv=%q", err, inv.args, inv.env)
		}
	})
}

// failFuzz fails t with the given message, unless sig is in skips, in which
// case it skips t.
func failFuzz(t *testing.T, skips map[string]struct{}, sig, format string, args ...any) {
	t.Helper()

	if _, ok := skips[sig]; ok {
		t.Skipf("known failure: %s", sig)
	}

	t.Logf("fuzz signature: %s", sig)
	require.Failf(t, "fuzz failure", format, args...)
}

// fuzzPanicSignature names a panic by its message and the innermost
// Terragrunt frame that raised it, so two inputs tripping the same bug share
// a signature. Digits and anything from the first quote or parenthesis on are
// dropped from the message, since that is where input data lands.
//
// .github/scripts/fuzz/full-cli.sh derives the same signature from the output
// of a crashed worker; keep the two in step.
func fuzzPanicSignature(msg string, stack []byte) string {
	msg, _, _ = strings.Cut(msg, "\n")
	msg = fuzzMessageDataRe.ReplaceAllString(fuzzDigitsRe.ReplaceAllString(msg, "N"), "")

	return "panic: " + strings.TrimSpace(msg) + " at " + fuzzPanicFrame(stack)
}

// fuzzPanicFrame returns the first Terragrunt function after the last
// `panic(` frame in stack, or after the start when there is none.
func fuzzPanicFrame(stack []byte) string {
	lines := strings.Split(string(stack), "\n")

	start := 0

	for i, line := range lines {
		if strings.HasPrefix(line, "panic(") {
			start = i + 1
		}
	}

	for _, line := range lines[start:] {
		if strings.HasPrefix(line, fuzzModuleFramePrefix) {
			return fuzzFrameArgsRe.ReplaceAllString(line, "")
		}
	}

	return "unknown"
}

// loadFuzzSkipList reads the signatures in the file [EnvFuzzSkipList] names.
// Blank lines and lines starting with # are ignored.
func loadFuzzSkipList(f *testing.F) map[string]struct{} {
	f.Helper()

	skips := map[string]struct{}{}

	path := os.Getenv(EnvFuzzSkipList)
	if path == "" {
		return skips
	}

	contents, err := vfs.ReadFile(vfs.NewOSFS(), path)
	if errors.Is(err, fs.ErrNotExist) {
		return skips
	}

	require.NoError(f, err)

	for line := range strings.Lines(string(contents)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		skips[line] = struct{}{}
	}

	return skips
}

// fuzzCountingWriter counts the bytes written to it and drops them. It is
// safe for the concurrent writes run --all makes.
type fuzzCountingWriter struct {
	n atomic.Int64
}

func (w *fuzzCountingWriter) Write(p []byte) (int, error) {
	w.n.Add(int64(len(p)))

	return len(p), nil
}

// fuzzConsumer reads structured values from a fuzz input. Subprocess and HTTP
// handlers share it and may be called concurrently, so every read holds mu.
// Once the input runs out, every read returns the zero value, so a short
// input still derives a whole invocation.
type fuzzConsumer struct {
	r  *bytes.Reader
	mu sync.Mutex
}

func newFuzzConsumer(data []byte) *fuzzConsumer {
	return &fuzzConsumer{r: bytes.NewReader(data)}
}

func (c *fuzzConsumer) nextByte() byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	b, err := c.r.ReadByte()
	if err != nil {
		return 0
	}

	return b
}

func (c *fuzzConsumer) uint16() uint16 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var buf [2]byte
	if n, err := c.r.Read(buf[:]); err != nil || n < len(buf) {
		return 0
	}

	return binary.LittleEndian.Uint16(buf[:])
}

func (c *fuzzConsumer) intN(n int) int {
	if n <= 0 {
		return 0
	}

	return int(c.uint16()) % n
}

func (c *fuzzConsumer) boolean() bool {
	return c.nextByte()&1 == 1
}

func (c *fuzzConsumer) choose(pool []string) string {
	if len(pool) == 0 {
		return ""
	}

	return pool[c.intN(len(pool))]
}

// word returns up to maxLen bytes from a small identifier alphabet, for slots
// that have to stay valid inside a quoted HCL string or a path.
func (c *fuzzConsumer) word(maxLen int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz_-0123456789"

	buf := make([]byte, c.intN(maxLen+1))
	for i := range buf {
		buf[i] = alphabet[int(c.nextByte())%len(alphabet)]
	}

	return string(buf)
}

// raw returns up to maxLen bytes taken from the input unchanged.
func (c *fuzzConsumer) raw(maxLen int) []byte {
	buf := make([]byte, c.intN(maxLen+1))
	for i := range buf {
		buf[i] = c.nextByte()
	}

	return buf
}

// fuzzCommand is one invocable command path and every flag it accepts.
type fuzzCommand struct {
	path  []string
	flags []fuzzFlag
}

type fuzzFlag struct {
	defaultVal string
	names      []string
	takesValue bool
}

// fuzzVocab is what an invocation is assembled from: the command tree, the
// env vars its flags read, and pools of plausible values.
type fuzzVocab struct {
	commands []fuzzCommand
	envKeys  []string
	values   []string
}

// fuzzInvocation is one derived run of the CLI.
type fuzzInvocation struct {
	files   map[string][]byte
	env     map[string]string
	stdin   string
	command []string
	args    []string
}

// newFuzzVocab walks the command tree of a real app, so a new command or
// flag is fuzzed without touching this file.
func newFuzzVocab(f *testing.F) *fuzzVocab {
	f.Helper()

	app := cli.NewApp(logger.CreateLogger(), options.NewTerragruntOptions(vexec.NewNoSpawnExec()), venvtest.New())

	vocab := &fuzzVocab{values: fuzzValuePool()}
	envKeys := map[string]struct{}{}

	for _, key := range fuzzExtraEnvKeys {
		envKeys[key] = struct{}{}
	}

	global := fuzzFlagsOf(app.Flags, envKeys)
	vocab.walk(app.Commands, nil, global, envKeys)

	vocab.envKeys = slices.Sorted(maps.Keys(envKeys))

	require.NotEmpty(f, vocab.commands, "the command tree has no commands")

	return vocab
}

func (vocab *fuzzVocab) walk(cmds clihelper.Commands, parent []string, inherited []fuzzFlag, envKeys map[string]struct{}) {
	for _, cmd := range cmds {
		path := append(slices.Clone(parent), cmd.Name)
		if _, ok := fuzzExcludedCommands[strings.Join(path, " ")]; ok {
			continue
		}

		flags := append(slices.Clone(inherited), fuzzFlagsOf(cmd.Flags, envKeys)...)

		vocab.commands = append(vocab.commands, fuzzCommand{path: path, flags: flags})
		vocab.walk(cmd.Subcommands, path, flags, envKeys)
	}
}

func fuzzFlagsOf(flags clihelper.Flags, envKeys map[string]struct{}) []fuzzFlag {
	out := make([]fuzzFlag, 0, len(flags))

	for _, flag := range flags {
		for _, key := range flag.GetEnvVars() {
			envKeys[key] = struct{}{}
		}

		out = append(out, fuzzFlag{
			names:      flag.Names(),
			takesValue: flag.TakesValue(),
			defaultVal: flag.GetDefaultText(),
		})
	}

	return out
}

func (vocab *fuzzVocab) invocation(c *fuzzConsumer, worlds []fuzzWorld) fuzzInvocation {
	cmd := vocab.commands[c.intN(len(vocab.commands))]
	world := worlds[c.intN(len(worlds))]

	files := world.build(c)
	fuzzMutateOneFile(c, files)

	args := slices.Clone(cmd.path)

	for range c.intN(fuzzMaxFlags + 1) {
		if len(cmd.flags) == 0 {
			break
		}

		args = append(args, vocab.flagArgs(c, cmd.flags[c.intN(len(cmd.flags))])...)
	}

	for range c.intN(fuzzMaxPositionals + 1) {
		args = append(args, vocab.value(c))
	}

	if c.boolean() {
		args = append(args, "--")
		for range c.intN(fuzzMaxTFArgs + 1) {
			args = append(args, c.choose(fuzzTFArgPool))
		}
	}

	env := map[string]string{}
	for range c.intN(fuzzMaxEnvEntries + 1) {
		env[c.choose(vocab.envKeys)] = vocab.value(c)
	}

	return fuzzInvocation{
		command: cmd.path,
		args:    args,
		env:     env,
		files:   files,
		stdin:   c.choose(fuzzStdinPool),
	}
}

// flagArgs renders a flag under one of its names and aliases, deprecated ones
// included, in either the `--name=value` or the `--name value` form.
func (vocab *fuzzVocab) flagArgs(c *fuzzConsumer, flag fuzzFlag) []string {
	name := "--" + c.choose(flag.names)
	if !flag.takesValue {
		return []string{name}
	}

	value := vocab.value(c)
	if flag.defaultVal != "" && c.boolean() {
		value = flag.defaultVal
	}

	if c.boolean() {
		return []string{name, value}
	}

	return []string{name + "=" + value}
}

// value picks a flag, env, or positional value: usually one from the pool,
// sometimes an identifier, sometimes arbitrary bytes.
func (vocab *fuzzVocab) value(c *fuzzConsumer) string {
	switch c.nextByte() % 8 {
	case 0:
		return c.word(fuzzMaxSlot)
	case 1:
		return string(c.raw(fuzzMaxSlot))
	default:
		return c.choose(vocab.values)
	}
}

// fuzzValuePool collects values that some flag accepts, so a flag that
// validates its value still gets past validation often.
func fuzzValuePool() []string {
	return slices.Concat([]string{
		"", "true", "false", "0", "1", "-1", "2", "10", "1s", "1m",
		"json", "text", "hcl", "csv", "raw", "tree", "long", "dag", "type", "alpha",
		"trace", "debug", "info", "warn", "error", "stderr", "stdout",
		"bare", "pretty", "key-value", "%time %level %msg", "%(color=red)%msg",
		"tofu", "terraform", "opentofu", "plan", "apply", "destroy", "output", "init", "validate",
		fuzzRoot,
		filepath.Join(fuzzRoot, "app"),
		filepath.Join(fuzzRoot, "db"),
		filepath.Join(fuzzRoot, "live", "foo"),
		filepath.Join(fuzzRoot, "terragrunt.hcl"),
		filepath.Join(fuzzRoot, "out.json"),
		".", "..", "./app", "../outside", "app", "*", "**",
		"{./app}", "./**", "name=app", "type=unit", "type=stack", "!./db", "app...", "...db",
		"[HEAD]", "[main...HEAD]", "reading=root.hcl", "source=./mod", "./app | ./db",
		"region=us-east-1", "key=value", "a=b,c=d",
		"https://registry.opentofu.org", "git::https://example.com/repo.git//mod?ref=v1.0.0",
		"s3::https://s3.amazonaws.com/bucket/key", "tfr:///hashicorp/null/aws?version=3.0.0",
		"registry.opentofu.org/hashicorp/null", "hashicorp/null",
		"arn:aws:iam::123456789012:role/fuzz", "us-east-1",
	}, experiment.NewExperiments().Names(), controls.New().Names())
}

// fuzzExtraEnvKeys are variables no flag declares but that the code reads,
// mostly through the cloud SDKs and the OpenTofu environment.
var fuzzExtraEnvKeys = []string{
	"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_PROFILE",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
	"AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_PROJECT", "GOOGLE_OAUTH_ACCESS_TOKEN",
	"ARM_SUBSCRIPTION_ID", "ARM_TENANT_ID", "ARM_CLIENT_ID",
	"HOME", "PATH", "USER", "TMPDIR",
	"TF_VAR_region", "TF_PLUGIN_CACHE_DIR", "TF_DATA_DIR", "TF_CLI_CONFIG_FILE",
	"TF_INPUT", "TF_IN_AUTOMATION", "TF_LOG",
	"GITHUB_TOKEN", "GIT_TERMINAL_PROMPT",
}

// fuzzTFArgPool is what follows `--`. The subprocess is in memory, so these
// only exercise Terragrunt's own argument handling.
var fuzzTFArgPool = []string{
	"plan", "apply", "destroy", "init", "output", "validate", "show", "state", "import",
	"-out=plan.bin", "plan.bin", "-input=false", "-no-color", "-auto-approve",
	"-refresh=false", "-target=null_resource.x", "-detailed-exitcode", "-lock=false",
	"-destroy", "-json", "-upgrade", "-var=name=fuzz", "-var-file=extra.tfvars", "-help",
}

var fuzzStdinPool = []string{"", "y\n", "n\n", "yes\n", "\n", "y\ny\ny\n"}

// fuzzMutateOneFile sometimes damages one file of the world: it truncates it,
// splices input bytes into it, or replaces it with input bytes. That is where
// the parsers see input no fixture holds.
func fuzzMutateOneFile(c *fuzzConsumer, files map[string][]byte) {
	op := c.nextByte() % 8
	if op > 2 || len(files) == 0 {
		return
	}

	paths := slices.Sorted(maps.Keys(files))
	path := paths[c.intN(len(paths))]
	contents := files[path]
	at := c.intN(len(contents) + 1)

	switch op {
	case 0:
		files[path] = contents[:at]
	case 1:
		files[path] = slices.Concat(contents[:at], c.raw(fuzzMaxSlot), contents[at:])
	case 2:
		files[path] = c.raw(fuzzMaxMalformedHCL)
	}
}

// fuzzSyntheticWorlds each aim at one part of the config pipeline.
var fuzzSyntheticWorlds = []func(c *fuzzConsumer) map[string][]byte{
	fuzzWorldUnit,
	fuzzWorldDependency,
	fuzzWorldStack,
	fuzzWorldIncludes,
	fuzzWorldErrors,
	fuzzWorldHooksAndGenerate,
	fuzzWorldFunctions,
}

// fuzzWorld builds the files one iteration starts with, keyed by absolute path.
type fuzzWorld struct {
	build func(c *fuzzConsumer) map[string][]byte
}

// newFuzzWorlds returns the synthetic layouts followed by one world per
// directory under test/fixtures.
func newFuzzWorlds(f *testing.F) []fuzzWorld {
	f.Helper()

	fixtures, err := loadFuzzFixtures(fuzzFixturesDir)
	require.NoError(f, err)

	worlds := make([]fuzzWorld, 0, len(fuzzSyntheticWorlds)+len(fixtures))
	for _, build := range fuzzSyntheticWorlds {
		worlds = append(worlds, fuzzWorld{build: build})
	}

	for _, files := range fixtures {
		worlds = append(worlds, fuzzWorld{build: func(*fuzzConsumer) map[string][]byte {
			return maps.Clone(files)
		}})
	}

	return worlds
}

// loadFuzzFixtures reads every top-level fixture directory into memory once,
// rebased onto fuzzRoot. Generated directories and large files are skipped.
func loadFuzzFixtures(dir string) ([]map[string][]byte, error) {
	osfs := vfs.NewOSFS()

	entries, err := vfs.ReadDir(osfs, dir)
	if err != nil {
		return nil, err
	}

	out := make([]map[string][]byte, 0, len(entries))

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		base := filepath.Join(dir, entry.Name())
		files := map[string][]byte{}

		err := vfs.WalkDir(osfs, base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if d.Name() == ".terraform" || d.Name() == util.TerragruntCacheDir {
					return fs.SkipDir
				}

				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			if !info.Mode().IsRegular() || info.Size() > fuzzMaxFixtureFile {
				return nil
			}

			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}

			contents, err := vfs.ReadFile(osfs, path)
			if err != nil {
				return err
			}

			files[filepath.Join(fuzzRoot, rel)] = contents

			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", base, err)
		}

		if len(files) > 0 {
			out = append(out, files)
		}
	}

	return out, nil
}

func fuzzFiles(files map[string]string) map[string][]byte {
	out := make(map[string][]byte, len(files))
	for rel, contents := range files {
		out[filepath.Join(fuzzRoot, filepath.FromSlash(rel))] = []byte(contents)
	}

	return out
}

const fuzzModule = `
variable "name" {
  type    = string
  default = "fuzz"
}

resource "null_resource" "x" {}

output "id" {
  value = var.name
}
`

func fuzzWorldUnit(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"terragrunt.hcl": fmt.Sprintf(`
locals {
  name = %q
}

terraform {
  source = "./mod"
}

inputs = {
  name = local.name
}
`, c.word(fuzzMaxSlot)),
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldDependency(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"root.hcl": fmt.Sprintf(`
locals {
  region = %q
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    path = "${path_relative_to_include()}/tofu.tfstate"
  }
}
`, c.word(fuzzMaxSlot)),
		"db/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "../mod"
}

inputs = {
  name = include.root.locals.region
}
`,
		"app/terragrunt.hcl": `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

dependency "db" {
  config_path = "../db"

  mock_outputs = {
    id = "mock"
  }
  mock_outputs_allowed_terraform_commands = ["plan", "validate"]
}

dependencies {
  paths = ["../db"]
}

terraform {
  source = "../mod"
}

inputs = {
  name = dependency.db.outputs.id
}
`,
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldStack(c *fuzzConsumer) map[string][]byte {
	name := orFuzzDefault(c.word(fuzzMaxSlot), "foo")

	return fuzzFiles(map[string]string{
		"terragrunt.stack.hcl": fmt.Sprintf(`
locals {
  env = "dev"
}

unit %[1]q {
  source = "./units/unit"
  path   = "live/%[1]s"
  values = {
    name = local.env
  }
}

unit "bar" {
  source = "./units/unit"
  path   = "live/bar"
}

stack "nested" {
  source = "./stacks/nested"
  path   = "nested"
}
`, name),
		"stacks/nested/terragrunt.stack.hcl": `
unit "inner" {
  source = "../../units/unit"
  path   = "inner"
}
`,
		"units/unit/terragrunt.hcl": `
terraform {
  source = "../../mod"
}

inputs = {
  name = try(values.name, "none")
}
`,
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldIncludes(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"root.hcl": fmt.Sprintf(`
locals {
  env = %q
}

inputs = {
  env = local.env
}
`, c.word(fuzzMaxSlot)),
		"app/region.hcl": `
locals {
  region = "us-east-1"
}
`,
		"app/terragrunt.hcl": `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

include "region" {
  path   = "region.hcl"
  expose = true
}

terraform {
  source = "../mod"
}

inputs = {
  name = include.region.locals.region
}
`,
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldErrors(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"terragrunt.hcl": fmt.Sprintf(`
terraform {
  source = "./mod"
}

errors {
  retry "transient" {
    retryable_errors   = [".*timeout.*", ".*%s.*"]
    max_attempts       = %d
    sleep_interval_sec = 0
  }

  ignore "known" {
    ignorable_errors = [".*ignore.*"]
    message          = "ignored"
  }
}
`, c.word(fuzzMaxSlot), 1+c.intN(3)),
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldHooksAndGenerate(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"terragrunt.hcl": fmt.Sprintf(`
terraform {
  source = "./mod"

  before_hook "before" {
    commands = ["plan", "apply"]
    execute  = ["echo", %q]
  }

  after_hook "after" {
    commands     = ["plan", "apply"]
    execute      = ["echo", "after"]
    run_on_error = true
  }

  error_hook "error" {
    commands  = ["plan", "apply"]
    execute   = ["echo", "error"]
    on_errors = [".*"]
  }

  extra_arguments "vars" {
    commands  = get_terraform_commands_that_need_vars()
    arguments = ["-var", "name=fuzz"]
  }
}

generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents  = "provider \"null\" {}"
}

feature "flag" {
  default = false
}

exclude {
  if      = feature.flag.value
  actions = ["plan"]
}
`, c.word(fuzzMaxSlot)),
		"mod/main.tf": fuzzModule,
	})
}

func fuzzWorldFunctions(c *fuzzConsumer) map[string][]byte {
	return fuzzFiles(map[string]string{
		"terragrunt.hcl": fmt.Sprintf(`
locals {
  word     = %q
  vars     = read_terragrunt_config(find_in_parent_folders("vars.hcl"))
  cmd      = run_cmd("--terragrunt-quiet", "echo", local.word)
  env      = get_env("TG_FUZZ", "fallback")
  platform = get_platform()
  repo     = get_repo_root()
  path     = path_relative_from_include()
  account  = get_aws_account_id()
  sops     = sops_decrypt_file("secrets.json")
  yaml     = yamldecode(file("data.yaml"))
  merged   = merge(local.vars.locals, { word = local.word })
}

terraform {
  source = "./mod"
}

inputs = local.merged
`, c.word(fuzzMaxSlot)),
		"vars.hcl":     `locals { region = "us-east-1" }`,
		"data.yaml":    "key: value\nlist: [1, 2]\n",
		"secrets.json": `{"secret": "value"}`,
		"mod/main.tf":  fuzzModule,
	})
}

func orFuzzDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}

	return s
}

// fuzzLookPath finds every binary, so commands reach the point of running it.
func fuzzLookPath(file string) (string, error) {
	return "/usr/bin/" + file, nil
}

// fuzzExecHandler answers every subprocess from the input: either a reply
// shaped like the real binary's, so the caller keeps going, or an exit code
// and output taken from the input.
func fuzzExecHandler(c *fuzzConsumer) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if c.nextByte()%4 != 0 {
			return fuzzPlausibleExec(&inv)
		}

		return vexec.Result{
			ExitCode: int(c.nextByte() % 3),
			Stdout:   c.raw(fuzzMaxSubprocOut),
			Stderr:   c.raw(fuzzMaxSubprocOut),
		}
	}
}

func fuzzPlausibleExec(inv *vexec.Invocation) vexec.Result {
	name := filepath.Base(inv.Name)

	switch {
	case (name == "tofu" || name == "terraform") && slices.ContainsFunc(inv.Args, isFuzzVersionArg):
		return vexec.Result{Stdout: []byte("OpenTofu v1.10.0\non linux_amd64\n")}
	case (name == "tofu" || name == "terraform") && slices.Contains(inv.Args, tf.CommandNameOutput):
		return vexec.Result{Stdout: []byte(`{"id":{"sensitive":false,"type":"string","value":"fuzz"}}`)}
	case name == "git" && slices.Contains(inv.Args, "--show-toplevel"):
		return vexec.Result{Stdout: []byte(fuzzRoot + "\n")}
	case name == "git" && slices.Contains(inv.Args, "remote"):
		return vexec.Result{Stdout: []byte("https://github.com/gruntwork-io/terragrunt.git\n")}
	}

	return vexec.Result{}
}

func isFuzzVersionArg(arg string) bool {
	return arg == tf.FlagNameVersion || arg == tf.CommandNameVersion
}

// fuzzHTTPHandler answers every outbound request, the cloud SDKs' included,
// from the input. It leans towards `{}` so JSON decoders pass and the caller
// gets further.
func fuzzHTTPHandler(c *fuzzConsumer) vhttp.Handler {
	return func(_ context.Context, _ *http.Request) (*http.Response, error) {
		status := []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError}[c.nextByte()%3]

		body := []byte("{}")
		if c.boolean() {
			body = c.raw(fuzzMaxHTTPBody)
		}

		return vhttp.Respond(status, body, nil), nil
	}
}
