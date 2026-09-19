package test_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// canonicalProviderVersions is the one version of each provider that configurations in the
// repository install. CI caches providers so registry outages don't fail runs, and one version per
// provider keeps that cache small.
var canonicalProviderVersions = map[string]string{
	"datadog/datadog":      "3.85.0",
	"hashicorp/aws":        "6.56.0",
	"hashicorp/azurerm":    "4.58.0",
	"hashicorp/consul":     "2.22.1",
	"hashicorp/external":   "2.3.5",
	"hashicorp/google":     "6.50.0",
	"hashicorp/helm":       "3.1.1",
	"hashicorp/kubernetes": "2.38.0",
	"hashicorp/local":      "2.6.1",
	"hashicorp/nomad":      "2.5.2",
	"hashicorp/null":       "3.2.4",
	"hashicorp/random":     "3.8.0",
	"hashicorp/time":       "0.14.2",
	"hashicorp/tls":        "4.1.0",
	"hashicorp/vault":      "5.6.0",
	"integrations/github":  "6.10.2",
}

// providerRule is one requirement TestProviderVersionsMatchCanonicalVersions places on
// configuration.
type providerRule int

const (
	// ruleCanonicalVersion requires every pinned or locked version to be the canonical one.
	ruleCanonicalVersion providerRule = iota
	// rulePinned requires every provider a module uses to be pinned in required_providers.
	rulePinned
	// ruleOpenTofuSource requires sources to name registry.opentofu.org, so Terraform installs
	// from the OpenTofu registry too and reads the same lock entries.
	ruleOpenTofuSource
	// ruleLockEntry requires the lock file init reads to hold an OpenTofu registry entry for every
	// provider the configuration installs.
	ruleLockEntry
	// ruleNoTerraformRegistry forbids lock entries for registry.terraform.io.
	ruleNoTerraformRegistry
)

// providerExemption waives rules for a directory and everything below it.
type providerExemption struct {
	reason string
	rules  []providerRule
}

// providerVersionExemptions maps a directory, relative to the repository root, to the rules its
// configurations are exempt from.
var providerVersionExemptions = map[string]providerExemption{
	"docs/src/fixtures": {
		reason: "the docs tests run these only under OpenTofu, and the guides show sources the way users write them",
		rules:  []providerRule{ruleOpenTofuSource},
	},
	"test/fixtures/docs/02-overview": {
		reason: "the units source tfr:// modules whose unqualified sources Terraform resolves to registry.terraform.io, and the AWS with Latest Terraform job runs them",
		rules:  []providerRule{ruleNoTerraformRegistry},
	},
	"test/fixtures/provider-cache": {
		reason: "the provider cache tests check the lock files the provider cache writes",
		rules:  []providerRule{ruleLockEntry},
	},
	"test/fixtures/provider-cache/dependency": {
		reason: "TestTFTerragruntProviderCacheWithDependency checks each binary caches providers from its own default registry",
		rules:  []providerRule{ruleOpenTofuSource},
	},
	"test/fixtures/provider-cache/direct": {
		reason: "TestTFTerragruntProviderCache checks each binary caches providers from its own default registry",
		rules:  []providerRule{ruleOpenTofuSource},
	},
	"test/fixtures/provider-cache/lockfile-readonly": {
		reason: "the provider cache intercepts each binary's own default registry",
		rules:  []providerRule{ruleOpenTofuSource},
	},
	"test/fixtures/provider-cache/multiple-platforms": {
		reason: "TestTFTerragruntProviderCacheMultiplePlatforms checks each binary locks providers from its own default registry",
		rules:  []providerRule{ruleOpenTofuSource},
	},
	"test/fixtures/provider-cache/weak-constraint/app": {
		reason: "TestTFTerragruntProviderCacheWeakConstraint checks the lock file records a range constraint on cloudflare/cloudflare from each binary's own default registry",
		rules:  []providerRule{ruleCanonicalVersion, ruleOpenTofuSource},
	},
}

// skippedProviderVersionDirs are never scanned: they hold no configuration the test suite runs.
var skippedProviderVersionDirs = []string{
	".git",
	"internal/vendored",
	"node_modules",
}

// maxLocalModuleDepth bounds how deep local module calls are followed.
const maxLocalModuleDepth = 16

const (
	openTofuRegistry  = "registry.opentofu.org"
	terraformRegistry = "registry.terraform.io"
)

func TestProviderVersionsMatchCanonicalVersions(t *testing.T) {
	t.Parallel()

	scan := newProviderScan(os.DirFS(".."))

	dirs, err := configDirs(scan.repo)
	require.NoError(t, err)
	require.NoError(t, scan.parse(dirs))

	waived := map[string]map[providerRule]struct{}{}

	var violations []providerViolation

	for _, dir := range dirs {
		dirViolations, err := scan.violations(dir)
		require.NoError(t, err)

		for _, v := range dirViolations {
			key, exempt := exemptionFor(dir, v.rule)
			if !exempt {
				violations = append(violations, v)

				continue
			}

			if waived[key] == nil {
				waived[key] = map[providerRule]struct{}{}
			}

			waived[key][v.rule] = struct{}{}
		}
	}

	for key, exemption := range providerVersionExemptions {
		for _, rule := range exemption.rules {
			assert.Contains(t, waived[key], rule, "%s is exempt from %s but nothing there breaks it; remove the exemption", key, rule)
		}
	}

	for _, v := range violations {
		assert.Fail(t, v.message)
	}
}

// pinnedProvidersTF returns a terraform block pinning each of sources to its canonical version on
// the OpenTofu registry, for tests that write configuration instead of copying a fixture.
//
// Panics when a source has no entry in canonicalProviderVersions.
func pinnedProvidersTF(sources ...string) string {
	var b strings.Builder

	b.WriteString("terraform {\n  required_providers {\n")

	for _, source := range sources {
		version, ok := canonicalProviderVersions[source]
		if !ok {
			panic("pinnedProvidersTF: no canonical version for " + source)
		}

		_, name, _ := strings.Cut(source, "/")
		fmt.Fprintf(&b, "    %s = {\n      source  = %q\n      version = %q\n    }\n", name, openTofuRegistry+"/"+source, version)
	}

	b.WriteString("  }\n}\n\n")

	return b.String()
}

// String names the rule in failure messages.
func (r providerRule) String() string {
	switch r {
	case ruleCanonicalVersion:
		return "ruleCanonicalVersion"
	case rulePinned:
		return "rulePinned"
	case ruleOpenTofuSource:
		return "ruleOpenTofuSource"
	case ruleLockEntry:
		return "ruleLockEntry"
	case ruleNoTerraformRegistry:
		return "ruleNoTerraformRegistry"
	}

	panic(fmt.Sprintf("unknown providerRule %d", int(r)))
}

// exemptionFor returns the key of the exemption that waives rule for dir.
func exemptionFor(dir string, rule providerRule) (string, bool) {
	for key, exemption := range providerVersionExemptions {
		if isWithin(dir, key) && slices.Contains(exemption.rules, rule) {
			return key, true
		}
	}

	return "", false
}

// isWithin reports whether dir is ancestor or below it. Both are slash-separated fs.FS paths.
func isWithin(dir, ancestor string) bool {
	for ; dir != "."; dir = path.Dir(dir) {
		if dir == ancestor {
			return true
		}
	}

	return ancestor == "."
}

// isConfigFile reports whether name holds OpenTofu or Terragrunt configuration.
func isConfigFile(name string) bool {
	switch path.Ext(name) {
	case ".tf", ".tofu", ".hcl":
		return true
	}

	return false
}

// configDirs returns every directory holding OpenTofu configuration, a lock file, or a Terragrunt
// configuration.
func configDirs(repo fs.FS) ([]string, error) {
	dirs := map[string]struct{}{}

	err := fs.WalkDir(repo, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if slices.Contains(skippedProviderVersionDirs, p) || d.Name() == ".terraform" || d.Name() == ".terragrunt-cache" {
				return fs.SkipDir
			}

			return nil
		}

		if !isConfigFile(p) {
			return nil
		}

		dirs[path.Dir(p)] = struct{}{}

		return nil
	})

	return slices.Sorted(maps.Keys(dirs)), err
}

// providerViolation is one way a directory breaks a rule.
type providerViolation struct {
	message string
	rule    providerRule
}

// providerScan reads configuration from repo, parsing each directory once.
type providerScan struct {
	repo  fs.FS
	dirs  map[string]*providerDir
	stack map[string]struct{}
	// called holds every directory some module calls as a local child module.
	called map[string]struct{}
}

func newProviderScan(repo fs.FS) *providerScan {
	return &providerScan{
		repo:   repo,
		dirs:   map[string]*providerDir{},
		stack:  map[string]struct{}{},
		called: map[string]struct{}{},
	}
}

// parse reads every directory in dirs, so violations knows which are called as child modules.
func (s *providerScan) parse(dirs []string) error {
	for _, dir := range dirs {
		d, err := s.dir(dir)
		if err != nil {
			return err
		}

		for _, child := range d.children {
			s.called[child] = struct{}{}
		}
	}

	return nil
}

// providerDir is what the files directly in one directory declare.
type providerDir struct {
	// required holds the module's required_providers entries by local name.
	required map[string]requiredProvider
	// used holds the local names of providers the module's blocks reference.
	used map[string]struct{}
	// locked maps each lock file entry's address to its version; nil when there is no lock file.
	locked map[string]string
	// unitSource is the local module a terragrunt.hcl names as its source, if any.
	unitSource string
	// children lists the directories of local modules the module calls.
	children []string
	// violations holds what parsing the files found on its own.
	violations []providerViolation
	// unit reports whether the directory holds a terragrunt.hcl.
	unit bool
}

// requiredProvider is one entry of a required_providers block.
type requiredProvider struct {
	source  string
	version string
}

// violations checks dir against every rule.
func (s *providerScan) violations(dir string) ([]providerViolation, error) {
	d, err := s.dir(dir)
	if err != nil {
		return nil, err
	}

	violations := slices.Clone(d.violations)

	for local, required := range d.required {
		violations = append(violations, versionViolations(dir, required.source, required.version)...)
		violations = append(violations, sourceViolations(dir, local, required.source)...)
	}

	for local := range d.used {
		if _, required := d.required[local]; required {
			continue
		}

		if _, known := canonicalProviderVersions["hashicorp/"+local]; !known {
			continue
		}

		violations = append(violations, providerViolation{
			rule:    rulePinned,
			message: fmt.Sprintf("%s: uses hashicorp/%s without pinning its version in required_providers", dir, local),
		})
	}

	lock := path.Join(dir, tf.TerraformLockFile)

	for address, version := range d.locked {
		violations = append(violations, versionViolations(lock, address, version)...)

		if host, _, _ := splitProviderSource(address); host == terraformRegistry {
			violations = append(violations, providerViolation{
				rule:    ruleNoTerraformRegistry,
				message: fmt.Sprintf("%s: remove the %s entry; tests install from %s", lock, address, openTofuRegistry),
			})
		}
	}

	lockViolations, err := s.lockViolations(dir, d)
	if err != nil {
		return nil, err
	}

	return append(violations, lockViolations...), nil
}

// lockViolations reports each provider init installs in dir that the lock file it reads doesn't
// hold. A module reads its own lock file; a unit that has one replaces its source module's with it.
// A module only other modules call reads its caller's lock file, so it needs none of its own.
func (s *providerScan) lockViolations(dir string, d *providerDir) ([]providerViolation, error) {
	if _, called := s.called[dir]; called && !d.unit && d.locked == nil {
		return nil, nil
	}

	installs, err := s.installs(dir, 0)
	if err != nil {
		return nil, err
	}

	if d.unitSource != "" && d.locked != nil {
		source, err := s.installs(d.unitSource, 0)
		if err != nil {
			return nil, err
		}

		maps.Copy(installs, source)
	}

	var violations []providerViolation

	for _, source := range slices.Sorted(maps.Keys(installs)) {
		if _, ok := d.locked[openTofuRegistry+"/"+source]; ok {
			continue
		}

		violations = append(violations, providerViolation{
			rule:    ruleLockEntry,
			message: fmt.Sprintf("%s: no %s/%s entry", path.Join(dir, tf.TerraformLockFile), openTofuRegistry, source),
		})
	}

	return violations, nil
}

// installs returns the public providers init installs for the module in dir from the OpenTofu
// registry, including those of the local modules it calls, keyed by source without its hostname.
func (s *providerScan) installs(dir string, depth int) (map[string]struct{}, error) {
	if depth > maxLocalModuleDepth {
		return nil, fmt.Errorf("%s: local modules nest deeper than %d", dir, maxLocalModuleDepth)
	}

	installs := map[string]struct{}{}

	if _, cycle := s.stack[dir]; cycle {
		return installs, nil
	}

	s.stack[dir] = struct{}{}
	defer delete(s.stack, dir)

	d, err := s.dir(dir)
	if err != nil {
		return nil, err
	}

	for _, required := range d.required {
		host, source, public := splitProviderSource(required.source)
		if !public || host == terraformRegistry {
			continue
		}

		if _, known := canonicalProviderVersions[source]; known {
			installs[source] = struct{}{}
		}
	}

	for _, child := range d.children {
		childInstalls, err := s.installs(child, depth+1)
		if err != nil {
			return nil, err
		}

		maps.Copy(installs, childInstalls)
	}

	return installs, nil
}

// dir parses the files directly in dir, once. A directory that doesn't exist, such as the target
// of a module call a fixture deliberately breaks, declares nothing.
func (s *providerScan) dir(dir string) (*providerDir, error) {
	if d, ok := s.dirs[dir]; ok {
		return d, nil
	}

	d := &providerDir{required: map[string]requiredProvider{}, used: map[string]struct{}{}}
	s.dirs[dir] = d

	entries, err := fs.ReadDir(s.repo, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return d, nil
	}

	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		name := entry.Name()

		// Symlinks are skipped: their targets are scanned where they live.
		if !entry.Type().IsRegular() {
			continue
		}

		file := path.Join(dir, name)

		src, err := fs.ReadFile(s.repo, file)
		if err != nil {
			return nil, err
		}

		switch ext := path.Ext(name); {
		case name == tf.TerraformLockFile:
			d.addLockFile(file, src)
		case ext == ".tf" || ext == ".tofu":
			d.addModuleFile(dir, file, src)
		case ext == ".hcl":
			d.addTerragruntFile(dir, file, src)
		}
	}

	return d, nil
}

// addModuleFile records the providers file requires and uses, and the local modules it calls.
// Files that don't parse are skipped: several fixtures are deliberately invalid, and a file
// OpenTofu can't parse installs no providers.
func (d *providerDir) addModuleFile(dir, file string, src []byte) {
	parsed, diags := hclsyntax.ParseConfig(src, file, hcl.InitialPos)
	if diags.HasErrors() {
		return
	}

	for _, block := range parsed.Body.(*hclsyntax.Body).Blocks {
		if block.Type != "terraform" && len(block.Labels) == 0 {
			continue
		}

		switch block.Type {
		case "terraform":
			d.violations = append(d.violations, addRequiredProviders(d.required, file, block.Body)...)
		case "resource", "data", "ephemeral":
			local, _, _ := strings.Cut(block.Labels[0], "_")
			d.used[local] = struct{}{}
		case "provider":
			d.used[block.Labels[0]] = struct{}{}
		case "module":
			if child, ok := localSource(dir, block.Body); ok {
				d.children = append(d.children, child)
			}
		}
	}
}

// addLockFile records every entry of a lock file.
func (d *providerDir) addLockFile(file string, src []byte) {
	d.locked = map[string]string{}

	parsed, diags := hclsyntax.ParseConfig(src, file, hcl.InitialPos)
	if diags.HasErrors() {
		d.violations = append(d.violations, providerViolation{rule: ruleLockEntry, message: fmt.Sprintf("%s: %v", file, diags)})

		return
	}

	for _, block := range parsed.Body.(*hclsyntax.Body).Blocks {
		if block.Type != "provider" || len(block.Labels) == 0 {
			continue
		}

		attr, ok := block.Body.Attributes["version"]
		if !ok {
			continue
		}

		version, err := stringValue(attr.Expr, nil)
		if err != nil {
			d.violations = append(d.violations, providerViolation{rule: ruleCanonicalVersion, message: fmt.Sprintf("%s: %v", file, err)})

			continue
		}

		d.locked[strings.ToLower(block.Labels[0])] = version
	}
}

// addTerragruntFile records the module a terragrunt.hcl sources when it names a local path
// directly, and checks the required_providers entries in the configuration each generate block
// writes. Generated contents may interpolate the file's locals. A generated provider block without
// a version isn't reported: the module it lands in may pin it.
func (d *providerDir) addTerragruntFile(dir, file string, src []byte) {
	parsed, diags := hclsyntax.ParseConfig(src, file, hcl.InitialPos)
	if diags.HasErrors() {
		return
	}

	body := parsed.Body.(*hclsyntax.Body)
	ctx := &hcl.EvalContext{Variables: map[string]cty.Value{"local": literalLocals(body)}}

	unit := path.Base(file) == config.DefaultTerragruntConfigPath
	if unit {
		d.unit = true
	}

	for _, block := range body.Blocks {
		if block.Type == "terraform" && unit {
			if source, ok := localSource(dir, block.Body); ok {
				d.unitSource = source
			}
		}

		if block.Type != "generate" || len(block.Labels) == 0 {
			continue
		}

		attr, ok := block.Body.Attributes["contents"]
		if !ok {
			continue
		}

		contents, err := stringValue(attr.Expr, ctx)
		if err != nil {
			continue
		}

		d.addGeneratedConfig(fmt.Sprintf("%s (generate %q)", file, block.Labels[0]), contents)
	}
}

// addGeneratedConfig checks the required_providers entries in generated configuration.
func (d *providerDir) addGeneratedConfig(where, contents string) {
	generated, diags := hclsyntax.ParseConfig([]byte(contents), where, hcl.InitialPos)
	if diags.HasErrors() {
		return
	}

	required := map[string]requiredProvider{}

	for _, block := range generated.Body.(*hclsyntax.Body).Blocks {
		if block.Type == "terraform" {
			d.violations = append(d.violations, addRequiredProviders(required, where, block.Body)...)
		}
	}

	for local, r := range required {
		d.violations = append(d.violations, versionViolations(where, r.source, r.version)...)
		d.violations = append(d.violations, sourceViolations(where, local, r.source)...)
	}
}

// addRequiredProviders records into required the entries of every required_providers block in a
// terraform block.
func addRequiredProviders(required map[string]requiredProvider, file string, body *hclsyntax.Body) []providerViolation {
	var violations []providerViolation

	for _, block := range body.Blocks {
		if block.Type != "required_providers" {
			continue
		}

		for local, attr := range block.Body.Attributes {
			r, err := parseRequiredProvider(local, attr.Expr)
			if err != nil {
				violations = append(violations, providerViolation{
					rule:    ruleCanonicalVersion,
					message: fmt.Sprintf("%s: provider %q: %v", file, local, err),
				})

				continue
			}

			required[local] = r
		}
	}

	return violations
}

// parseRequiredProvider reads either form of a required_providers entry: an object with source and
// version, or the legacy bare version string.
func parseRequiredProvider(local string, expr hclsyntax.Expression) (requiredProvider, error) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		return requiredProvider{}, diags
	}

	required := requiredProvider{source: "hashicorp/" + local}

	if val.Type() == cty.String && val.IsKnown() && !val.IsNull() {
		required.version = val.AsString()

		return required, nil
	}

	if !val.Type().IsObjectType() || !val.IsKnown() || val.IsNull() {
		return requiredProvider{}, fmt.Errorf("unsupported required_providers entry of type %s", val.Type().FriendlyName())
	}

	for attr, dst := range map[string]*string{"source": &required.source, "version": &required.version} {
		if !val.Type().HasAttribute(attr) {
			continue
		}

		v := val.GetAttr(attr)
		if v.Type() != cty.String || !v.IsKnown() || v.IsNull() {
			return requiredProvider{}, fmt.Errorf("%s is not a string", attr)
		}

		*dst = v.AsString()
	}

	return required, nil
}

// localSource resolves a body's literal source attribute when it names a local path.
func localSource(dir string, body *hclsyntax.Body) (string, bool) {
	attr, ok := body.Attributes["source"]
	if !ok {
		return "", false
	}

	source, err := stringValue(attr.Expr, nil)
	if err != nil || (!strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../")) {
		return "", false
	}

	return path.Clean(path.Join(dir, strings.Replace(source, "//", "/", 1))), true
}

// stringValue evaluates expr, which must produce a known string.
func stringValue(expr hclsyntax.Expression, ctx *hcl.EvalContext) (string, error) {
	val, diags := expr.Value(ctx)
	if diags.HasErrors() {
		return "", diags
	}

	if val.Type() != cty.String || !val.IsKnown() || val.IsNull() {
		return "", fmt.Errorf("expected a string, got %s", val.Type().FriendlyName())
	}

	return val.AsString(), nil
}

// literalLocals evaluates the locals in body that need no evaluation context.
func literalLocals(body *hclsyntax.Body) cty.Value {
	locals := map[string]cty.Value{}

	for _, block := range body.Blocks {
		if block.Type != "locals" {
			continue
		}

		for name, attr := range block.Body.Attributes {
			val, diags := attr.Expr.Value(nil)
			if !diags.HasErrors() {
				locals[name] = val
			}
		}
	}

	return cty.ObjectVal(locals)
}

// versionViolations reports a version that isn't exactly the canonical version of source.
// Providers served from hosts other than the public registries are fakes the tests build, so they
// are skipped.
func versionViolations(where, source, version string) []providerViolation {
	_, source, public := splitProviderSource(source)
	if !public {
		return nil
	}

	canonical, known := canonicalProviderVersions[source]
	if !known {
		return []providerViolation{{rule: ruleCanonicalVersion, message: fmt.Sprintf("%s: %s has no entry in canonicalProviderVersions", where, source)}}
	}

	if strings.TrimSpace(strings.TrimPrefix(version, "=")) != canonical {
		return []providerViolation{{rule: ruleCanonicalVersion, message: fmt.Sprintf("%s: %s is %q, want %q", where, source, version, canonical)}}
	}

	return nil
}

// sourceViolations reports a public provider source that doesn't name the OpenTofu registry.
func sourceViolations(where, local, source string) []providerViolation {
	host, _, public := splitProviderSource(source)
	if !public || host == openTofuRegistry {
		return nil
	}

	return []providerViolation{{
		rule:    ruleOpenTofuSource,
		message: fmt.Sprintf("%s: provider %q has source %q; prefix it with %s/", where, local, source, openTofuRegistry),
	}}
}

// splitProviderSource splits source into its hostname, empty when source omits it, and the rest.
// It reports false for a source on a host other than the public registries.
func splitProviderSource(source string) (string, string, bool) {
	source = strings.ToLower(source)

	if strings.Count(source, "/") < 2 {
		return "", source, true
	}

	host, rest, _ := strings.Cut(source, "/")
	if host != openTofuRegistry && host != terraformRegistry {
		return "", "", false
	}

	return host, rest, true
}
