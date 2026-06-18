// Command check-test-tags verifies that every build-tagged Go test is reachable
// by at least one CI job.
//
// It models the integration-test matrix (build tags, target package set and the
// -run filter of each job) plus the base unit job, then for every test function
// guarded by a //go:build constraint that references a custom CI tag it checks
// whether some job compiles the file, covers its package and matches its name
// with the -run filter. Tests that no job runs are reported, as are custom build
// tags that no job enables.
//
// Pure platform constraints (GOOS, GOARCH, unix, cgo) are out of scope: those
// tests run via the platform's natural job or the base unit job.
//
// Usage:
//
//	go run ./.github/scripts/ci/check-test-tags
//
// Exit code is non-zero when any unreachable test or dead tag is found.
package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// workflowsDir holds the CI workflows whose matrices enable build tags.
const workflowsDir = ".github/workflows"

// allowlistRelPath holds intentional exceptions for the reachability check.
const allowlistRelPath = ".github/scripts/ci/check-test-tags/allowlist.txt"

// job is one resolved CI job: the tags it enables, the OS it runs on, the
// package target and the -run filter.
type job struct {
	name         string
	target       string
	run          string
	os           string
	customTags   map[string]bool
	dockerAppend bool
}

// testFunc is a single guarded test function discovered in the tree.
type testFunc struct {
	pkgDir string
	name   string
	file   string
	expr   constraint.Expr
	custom []string
}

// finding is one unreachable test reported to the user.
type finding struct {
	pkgDir string
	name   string
	file   string
	reason string
}

func main() {
	root, err := findRepoRoot()
	fail(err)

	jobs, err := loadJobs(filepath.Join(root, workflowsDir))
	fail(err)

	tests, err := collectGuardedTests(root)
	fail(err)

	allowTests, allowTags, err := loadAllowlist(filepath.Join(root, allowlistRelPath))
	fail(err)

	findings := analyze(jobs, tests, allowTests, allowTags)
	deadTags := deadTags(jobs, tests, allowTags)

	report(findings, deadTags)

	if len(findings) == 0 && len(deadTags) == 0 {
		fmt.Println("check-test-tags: OK - every build-tagged test is reachable by a CI job")
		return
	}

	os.Exit(1)
}

// analyze returns every guarded test that no job runs.
func analyze(jobs []job, tests []testFunc, allowTests, allowTags map[string]bool) []finding {
	var findings []finding

	for _, t := range tests {
		if allowTests[t.pkgDir+":"+t.name] {
			continue
		}

		if reachable(jobs, t) {
			continue
		}

		if allTagsAllowed(t.custom, allowTags) {
			continue
		}

		findings = append(findings, finding{
			pkgDir: t.pkgDir,
			name:   t.name,
			file:   t.file,
			reason: unreachableReason(jobs, t),
		})
	}

	sort.Slice(findings, func(i, k int) bool {
		if findings[i].file != findings[k].file {
			return findings[i].file < findings[k].file
		}
		return findings[i].name < findings[k].name
	})

	return findings
}

// reachable reports whether some job compiles, covers and selects the test.
func reachable(jobs []job, t testFunc) bool {
	for _, j := range jobs {
		if jobRunsTest(j, t) {
			return true
		}
	}
	return false
}

// jobRunsTest reports whether a single job compiles, covers and selects a test.
func jobRunsTest(j job, t testFunc) bool {
	if !t.expr.Eval(satisfies(j)) {
		return false
	}
	if !targetCovers(j.target, t.pkgDir) {
		return false
	}
	return runMatches(j.run, t.name)
}

// unreachableReason explains why no job runs the test, for the report.
func unreachableReason(jobs []job, t testFunc) string {
	var compilers []string

	for _, j := range jobs {
		if !t.expr.Eval(satisfies(j)) {
			continue
		}
		if !targetCovers(j.target, t.pkgDir) {
			continue
		}
		compilers = append(compilers, fmt.Sprintf("%q (-run %q)", j.name, j.run))
	}

	if len(compilers) == 0 {
		return "no job enables tags " + strings.Join(t.custom, ",") + " for this package"
	}

	return "compiled by " + strings.Join(compilers, ", ") + " but the name does not match the -run filter"
}

// deadTags returns custom build tags used by tests that no job ever enables.
func deadTags(jobs []job, tests []testFunc, allowTags map[string]bool) []string {
	enabled := map[string]bool{}
	for _, j := range jobs {
		for tag := range j.customTags {
			enabled[tag] = true
		}
		if j.dockerAppend {
			enabled["docker"] = true
		}
	}

	used := map[string]bool{}
	for _, t := range tests {
		for _, tag := range t.custom {
			used[tag] = true
		}
	}

	var dead []string
	for tag := range used {
		if enabled[tag] || allowTags[tag] {
			continue
		}
		dead = append(dead, tag)
	}

	sort.Strings(dead)
	return dead
}

// report prints findings to stdout in a stable, reviewable layout.
func report(findings []finding, deadTags []string) {
	if len(findings) > 0 {
		fmt.Printf("Unreachable build-tagged tests (%d):\n", len(findings))
		for _, f := range findings {
			fmt.Printf("  %s\n      %s :: %s\n", f.name, f.file, f.reason)
		}
		fmt.Println()
	}

	if len(deadTags) > 0 {
		fmt.Printf("Build tags enabled by no CI job (%d): %s\n\n", len(deadTags), strings.Join(deadTags, ", "))
	}
}

// satisfies returns the tag oracle for a job: platform tokens follow the job OS,
// custom tokens follow the job tag set, docker follows the auto-append rule.
func satisfies(j job) func(string) bool {
	platform := platformTags(j.os)

	return func(tag string) bool {
		// Tags set by go test flags such as -race are not modelled.
		if isPlatformToken(tag) {
			return platform[tag]
		}
		if tag == "docker" && j.dockerAppend {
			return true
		}
		return j.customTags[tag]
	}
}

// loadJobs discovers every matrix-based test job across the workflows directory
// and appends the base unit jobs. Discovering all workflows (not just the main
// integration matrix) keeps tags enabled in separate workflows, such as the OIDC
// suite, from looking unreachable.
func loadJobs(dir string) ([]job, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflows dir %s: %w", dir, err)
	}

	var jobs []job
	for _, entry := range entries {
		if entry.IsDir() || !isYAML(entry.Name()) {
			continue
		}
		fileJobs, err := jobsFromWorkflow(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, fileJobs...)
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("no matrix test jobs found under %s", dir)
	}

	return append(jobs, baseJobs()...), nil
}

// jobsFromWorkflow extracts the integration matrix entries from one workflow.
// Ubuntu integration runners get the docker tag appended at runtime, matching
// the Run Tests step, so they are modelled with dockerAppend set.
func jobsFromWorkflow(path string) ([]job, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow %s: %w", path, err)
	}

	var wf struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Integration []struct {
						Name   string `yaml:"name"`
						OS     string `yaml:"os"`
						Target string `yaml:"target"`
						Tags   string `yaml:"tags"`
						Run    string `yaml:"run"`
					} `yaml:"integration"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}

	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow %s: %w", path, err)
	}

	var jobs []job
	for _, j := range wf.Jobs {
		for _, e := range j.Strategy.Matrix.Integration {
			jobs = append(jobs, job{
				name:         e.Name,
				target:       e.Target,
				run:          e.Run,
				os:           e.OS,
				customTags:   splitTags(e.Tags),
				dockerAppend: e.OS == "ubuntu",
			})
		}
	}

	return jobs, nil
}

// collectGuardedTests walks the tree and returns every test function whose file
// carries a //go:build constraint that references at least one custom CI tag.
func collectGuardedTests(root string) ([]testFunc, error) {
	var tests []testFunc

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return skipDir(d.Name())
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fns, scanErr := scanFile(root, path)
		if scanErr != nil {
			return scanErr
		}

		tests = append(tests, fns...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return tests, nil
}

// scanFile returns the guarded test functions defined in one file.
func scanFile(root, path string) ([]testFunc, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	expr := buildConstraint(src)
	if expr == nil {
		return nil, nil
	}

	custom := customTokens(expr)
	if len(custom) == 0 {
		return nil, nil
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	pkgDir := relDir(root, path)

	var fns []testFunc
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !isTestFunc(fn) {
			continue
		}
		fns = append(fns, testFunc{
			pkgDir: pkgDir,
			name:   fn.Name.Name,
			file:   relDir(root, path) + "/" + filepath.Base(path),
			expr:   expr,
			custom: custom,
		})
	}

	return fns, nil
}

// buildConstraint extracts and parses the //go:build line, or returns nil.
func buildConstraint(src []byte) constraint.Expr {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "package ") {
			return nil
		}
		if !constraint.IsGoBuild(trimmed) {
			continue
		}
		expr, err := constraint.Parse(trimmed)
		if err != nil {
			return nil
		}
		return expr
	}
	return nil
}

// customTokens returns the sorted custom (non-platform) tags in an expression.
func customTokens(expr constraint.Expr) []string {
	seen := map[string]bool{}
	walkTags(expr, func(tag string) {
		if isPlatformToken(tag) {
			return
		}
		seen[tag] = true
	})

	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// walkTags invokes fn for every tag named in a build expression.
func walkTags(expr constraint.Expr, fn func(string)) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		fn(e.Tag)
	case *constraint.NotExpr:
		walkTags(e.X, fn)
	case *constraint.AndExpr:
		walkTags(e.X, fn)
		walkTags(e.Y, fn)
	case *constraint.OrExpr:
		walkTags(e.X, fn)
		walkTags(e.Y, fn)
	}
}

// isTestFunc reports whether a declaration is a `func TestXxx(t *testing.T)`.
func isTestFunc(fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	if fn.Name.Name == "TestMain" {
		return false
	}
	if !isTestName(fn.Name.Name) {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}

	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "T"
}

// isTestName reports whether name is a Go test name (Test + non-lowercase).
func isTestName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	rest := strings.TrimPrefix(name, "Test")
	if rest == "" {
		return false
	}
	return !(rest[0] >= 'a' && rest[0] <= 'z')
}

// targetCovers reports whether a `go test` target covers a package directory.
func targetCovers(target, pkgDir string) bool {
	t := strings.TrimPrefix(target, "./")
	if t == "..." || t == "" {
		return true
	}
	if strings.HasSuffix(t, "/...") {
		prefix := strings.TrimSuffix(t, "/...")
		return pkgDir == prefix || strings.HasPrefix(pkgDir, prefix+"/")
	}
	return pkgDir == t
}

// runMatches reports whether a -run filter selects a top-level test name.
func runMatches(run, name string) bool {
	if run == "" {
		return true
	}
	head := strings.SplitN(run, "/", 2)[0]
	re, err := regexp.Compile(head)
	if err != nil {
		return true
	}
	return re.MatchString(name)
}

// platformTags returns the platform tokens that are true on a given job OS.
func platformTags(os string) map[string]bool {
	switch os {
	case "windows":
		return map[string]bool{"windows": true, "amd64": true, "gc": true}
	case "macos":
		return map[string]bool{"darwin": true, "unix": true, "amd64": true, "gc": true, "cgo": true}
	default:
		return map[string]bool{"linux": true, "unix": true, "amd64": true, "gc": true, "cgo": true}
	}
}

// isPlatformToken reports whether a tag is a GOOS, GOARCH or toolchain token.
func isPlatformToken(tag string) bool {
	return platformTokens[tag]
}

// baseJobs models the base unit suite: no tags, no filter, both base runners.
func baseJobs() []job {
	return []job{
		{name: "Base Tests (ubuntu)", target: "./...", run: "", os: "ubuntu"},
		{name: "Base Tests (macos)", target: "./...", run: "", os: "macos"},
	}
}

// allTagsAllowed reports whether every custom tag of a test is allowlisted.
func allTagsAllowed(custom []string, allowTags map[string]bool) bool {
	if len(custom) == 0 {
		return false
	}
	for _, tag := range custom {
		if !allowTags[tag] {
			return false
		}
	}
	return true
}

// isYAML reports whether a filename is a YAML workflow file.
func isYAML(name string) bool {
	return strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")
}

// splitTags parses a comma-separated matrix tag string into a set.
func splitTags(tags string) map[string]bool {
	set := map[string]bool{}
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		set[tag] = true
	}
	return set
}

// loadAllowlist reads intentional exceptions; a missing file is not an error.
// A "tag:NAME" line marks a build tag that is deliberately enabled by no CI job
// (a manual or local-only suite); a "<package dir>:<TestName>" line, for example
// "internal/foo:TestBar", excuses one test.
func loadAllowlist(path string) (tests, tags map[string]bool, err error) {
	tests = map[string]bool{}
	tags = map[string]bool{}

	data, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		return tests, tags, nil
	}
	if readErr != nil {
		return nil, nil, fmt.Errorf("reading allowlist %s: %w", path, readErr)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if tag, ok := strings.CutPrefix(line, "tag:"); ok {
			tags[strings.TrimSpace(tag)] = true
			continue
		}
		tests[line] = true
	}

	return tests, tags, nil
}

// findRepoRoot walks up from the working directory to the module root.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// relDir returns the slash-separated package directory of a file under root.
func relDir(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return filepath.ToSlash(filepath.Dir(path))
	}
	return filepath.ToSlash(rel)
}

// skipDir reports whether a directory should be skipped during the walk.
func skipDir(name string) error {
	switch name {
	case "vendor", "node_modules", "testdata":
		return filepath.SkipDir
	}
	if strings.HasPrefix(name, ".") && name != "." {
		return filepath.SkipDir
	}
	return nil
}

// fail aborts with a clear message on a fatal setup error.
func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "check-test-tags:", err)
	os.Exit(2)
}

// platformTokens are GOOS, GOARCH and toolchain tags resolved by the platform,
// not by CI build tags. Sourced from the Makefile IGNORE_TAGS list.
var platformTokens = func() map[string]bool {
	tokens := []string{
		"windows", "linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly",
		"solaris", "plan9", "js", "wasip1", "aix", "android", "illumos", "ios",
		"386", "amd64", "arm", "arm64", "mips", "mips64", "mips64le", "mipsle",
		"ppc64", "ppc64le", "riscv64", "s390x", "wasm", "loong64",
		"unix", "cgo", "gc", "gccgo", "race", "msan", "asan", "boringcrypto", "purego",
	}
	set := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		set[tok] = true
	}
	return set
}()
