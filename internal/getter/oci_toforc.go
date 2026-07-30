package getter

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// Env vars OpenTofu reads to locate its CLI config, in the order it checks them.
const (
	envTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
	envTerraformConfig = "TERRAFORM_CONFIG"
)

// windowsGOOS is the platform whose CLI config lives under %APPDATA%.
const windowsGOOS = "windows"

// ociTofuRepoCredential is one decoded oci_credentials block: its registry and
// repository-path prefix, plus either a static credential or a helper suffix.
type ociTofuRepoCredential struct {
	registryDomain   string
	repositoryPrefix string
	cred             auth.Credential
	helper           string
}

// matches reports whether this block serves hostport/repositoryName. The label
// is a repository address prefix, so an empty repository prefix matches the
// whole registry and a non-empty one matches on a path-segment boundary.
func (c *ociTofuRepoCredential) matches(hostport, repositoryName string) bool {
	if c.registryDomain != ociCanonicalAuthKey(hostport) {
		return false
	}

	if c.repositoryPrefix == "" {
		return true
	}

	return repositoryName == c.repositoryPrefix ||
		strings.HasPrefix(repositoryName, c.repositoryPrefix+"/")
}

// specificity ranks this block the same way an ambient key ranks: domain level
// plus one per repository-path segment the label pins.
func (c *ociTofuRepoCredential) specificity() int {
	if c.repositoryPrefix == "" {
		return ociDomainSpecificity
	}

	return ociDomainSpecificity + strings.Count(c.repositoryPrefix, "/") + 1
}

// ociTofuCredentials is the decoded OCI subset of an OpenTofu CLI config: its
// oci_credentials blocks, the oci_default_credentials fallback helper, and
// whether ambient discovery is enabled.
type ociTofuCredentials struct {
	defaultHelper   string
	configDir       string
	configFiles     []string
	repos           []ociTofuRepoCredential
	discoverAmbient bool
	configFilesSet  bool
	hasDefault      bool
}

// ociTofuDefaults is the decoded oci_default_credentials block.
type ociTofuDefaults struct {
	helper          string
	configFiles     []string
	discoverAmbient bool
	configFilesSet  bool
}

// errOCIInvalidHelperName reports a credential helper name that is empty or
// contains a path separator, so it could execute a non-PATH binary.
var errOCIInvalidHelperName = errors.New("credential helper name must not be empty or contain a path separator")

// errOCIMissingCredentialStyle reports an oci_credentials block configuring no credential at all.
var errOCIMissingCredentialStyle = errors.New(
	"oci_credentials block must configure basic auth, OAuth tokens, or a helper",
)

// errOCIMultipleCredentialStyles reports an oci_credentials block configuring
// more than one of basic auth, OAuth, or a helper.
var errOCIMultipleCredentialStyles = errors.New("oci_credentials block must configure at most one credential style")

// errOCIIncompleteBasicCredential reports an oci_credentials block missing a username or password.
var errOCIIncompleteBasicCredential = errors.New("oci_credentials basic auth requires both a username and a password")

// errOCIIncompleteOAuthCredential reports an oci_credentials block missing an access or refresh token.
var errOCIIncompleteOAuthCredential = errors.New(
	"oci_credentials oauth requires both an access_token and a refresh_token",
)

// errOCIHelperWithRepositoryPath reports a helper on a repository-scoped label, which tofu rejects.
var errOCIHelperWithRepositoryPath = errors.New(
	"oci_credentials docker_credentials_helper cannot be used with a repository path",
)

// errOCIAmbientFilesWithoutDiscovery reports docker_style_config_files set while ambient discovery is off.
var errOCIAmbientFilesWithoutDiscovery = errors.New(
	"oci_default_credentials docker_style_config_files requires discover_ambient_credentials to be enabled",
)

// errOCIDuplicateDefaultBlock reports a second oci_default_credentials block, which tofu rejects.
var errOCIDuplicateDefaultBlock = errors.New("at most one oci_default_credentials block is allowed")

// errOCIDuplicateRepoBlock reports two oci_credentials blocks sharing a label, which tofu rejects.
var errOCIDuplicateRepoBlock = errors.New("duplicate oci_credentials block")

// errOCIEmptyCredentialValue reports a credential argument set to an empty string.
var errOCIEmptyCredentialValue = errors.New("oci_credentials values must not be empty")

// errOCILabelNotRepositoryAddress reports a label that is not a bare registry domain and repository path.
var errOCILabelNotRepositoryAddress = errors.New(
	"oci_credentials label must be a registry domain with an optional repository path, without a URL scheme",
)

// ociValidHelperName reports whether name is a safe docker-credential suffix:
// non-empty and free of path separators, matching OpenTofu's validation.
func ociValidHelperName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`)
}

// loadOCITofuCredentials reads and merges the CLI config's OCI blocks. A file
// that is absent is skipped, but one that exists and cannot be read or parsed
// is an error, so invalid configuration can never silently widen credentials.
func loadOCITofuCredentials(l log.Logger, v *venv.Venv) (ociTofuCredentials, error) {
	merged := ociTofuCredentials{discoverAmbient: true}
	seenRepos := make(map[string]struct{})

	for _, path := range ociTofuConfigSources(l, v) {
		data, err := vfs.ReadFile(v.FS, path)
		if err != nil {
			return ociTofuCredentials{}, fmt.Errorf("reading OpenTofu CLI config %s: %w", path, err)
		}

		one, err := decodeOCITofuCredentials(data, path)
		if err != nil {
			return ociTofuCredentials{}, fmt.Errorf("reading OpenTofu CLI config %s: %w", path, err)
		}

		if err := mergeOCITofuCredentials(&merged, &one, path, seenRepos); err != nil {
			return ociTofuCredentials{}, err
		}
	}

	return merged, nil
}

// mergeOCITofuCredentials folds one decoded source into merged, rejecting a
// repository label or default block already declared by an earlier source, as
// OpenTofu rejects duplicates outright.
func mergeOCITofuCredentials(
	merged, one *ociTofuCredentials,
	path string,
	seenRepos map[string]struct{},
) error {
	for _, repo := range one.repos {
		key := repo.registryDomain + "/" + repo.repositoryPrefix
		if _, dup := seenRepos[key]; dup {
			return fmt.Errorf("%w %q in %s", errOCIDuplicateRepoBlock, key, path)
		}

		seenRepos[key] = struct{}{}

		merged.repos = append(merged.repos, repo)
	}

	if !one.hasDefault {
		return nil
	}

	if merged.hasDefault {
		return fmt.Errorf("%w: %s", errOCIDuplicateDefaultBlock, path)
	}

	merged.hasDefault = true
	merged.defaultHelper = one.defaultHelper
	merged.discoverAmbient = one.discoverAmbient
	merged.configFiles = one.configFiles
	merged.configFilesSet = one.configFilesSet
	// Relative docker_style_config_files resolve against the declaring file.
	merged.configDir = filepath.Dir(path)

	return nil
}

// ociTofuConfigSources lists the CLI config files to read, in OpenTofu's order:
// the selected config file, then the *.tfrc and *.tfrc.json fragments in the
// config directory. An explicit env override suppresses the directory, matching
// OpenTofu's "doing something special" interpretation of that variable.
func ociTofuConfigSources(l log.Logger, v *venv.Venv) []string {
	var sources []string

	if path := ociTofuConfigPath(l, v); path != "" {
		if _, err := v.FS.Stat(path); err == nil {
			sources = append(sources, path)
		}
	}

	if v.Env[envTFCLIConfigFile] != "" || v.Env[envTerraformConfig] != "" {
		return sources
	}

	return append(sources, ociTofuConfigFragments(l, v)...)
}

// ociTofuConfigFragments returns the *.tfrc and *.tfrc.json files in OpenTofu's
// config directory, in filename order so merging is deterministic.
func ociTofuConfigFragments(l log.Logger, v *venv.Venv) []string {
	dir := ociTofuConfigDir(v)
	if dir == "" {
		return nil
	}

	info, err := v.FS.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	// ReadDirEntries sorts by name, so merging stays deterministic.
	entries, err := vfs.ReadDirEntries(v.FS, dir)
	if err != nil {
		l.Warnf("Skipping unreadable OpenTofu CLI config directory %s: %v", dir, err)

		return nil
	}

	var fragments []string

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".tfrc") && !strings.HasSuffix(name, ".tfrc.json") {
			continue
		}

		fragments = append(fragments, filepath.Join(dir, name))
	}

	return fragments
}

// ociTofuConfigDir resolves OpenTofu's CLI config directory for the platform.
func ociTofuConfigDir(v *venv.Venv) string {
	home, err := v.Platform.UserHomeDir()
	if err != nil {
		home = ""
	}

	if v.Platform.GOOS == windowsGOOS {
		if appData := v.Env["APPDATA"]; appData != "" {
			return filepath.Join(appData, "terraform.d")
		}

		return ""
	}

	if home != "" {
		legacy := filepath.Join(home, ".terraform.d")
		if _, statErr := v.FS.Stat(legacy); statErr == nil {
			return legacy
		}
	}

	if xdg := v.Env["XDG_CONFIG_HOME"]; xdg != "" {
		return filepath.Join(xdg, "opentofu")
	}

	if home == "" {
		return ""
	}

	return filepath.Join(home, ".terraform.d")
}

// ociTofuConfigPath resolves the CLI config path OpenTofu would use: the
// TF_CLI_CONFIG_FILE or TERRAFORM_CONFIG override, else the first of
// ~/.tofurc, ~/.terraformrc that exists.
func ociTofuConfigPath(l log.Logger, v *venv.Venv) string {
	for _, name := range []string{envTFCLIConfigFile, envTerraformConfig} {
		override := v.Env[name]
		if override == "" {
			continue
		}

		if _, err := v.FS.Stat(override); err != nil {
			l.Warnf("OpenTofu CLI config %s set by %s cannot be read: %v", override, name, err)
		}

		return override
	}

	for _, candidate := range ociTofuConfigCandidates(v) {
		if _, statErr := v.FS.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	return ""
}

// parseOCITofuConfig parses the CLI config, picking HCL's JSON parser for the
// JSON form OpenTofu also accepts. tofu detects JSON from the content, so a
// leading brace counts even when the path carries no .json suffix.
func parseOCITofuConfig(data []byte, path string) (*hcl.File, hcl.Diagnostics) {
	if strings.HasSuffix(path, ".json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return hcljson.Parse(data, path)
	}

	return hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
}

// ociTofuConfigCandidates lists OpenTofu's default CLI-config locations for the
// injected platform, in the order OpenTofu searches them.
func ociTofuConfigCandidates(v *venv.Venv) []string {
	home, err := v.Platform.UserHomeDir()
	if err != nil {
		home = ""
	}

	var paths []string

	// On Windows tofu reads only tofu.rc and terraform.rc under %APPDATA%.
	if v.Platform.GOOS == windowsGOOS {
		if appData := v.Env["APPDATA"]; appData != "" {
			return []string{
				filepath.Join(appData, "tofu.rc"),
				filepath.Join(appData, "terraform.rc"),
			}
		}

		return nil
	}

	if home != "" {
		paths = append(paths, filepath.Join(home, ".tofurc"), filepath.Join(home, ".terraformrc"))
	}

	// tofu falls back to XDG only when XDG_CONFIG_HOME is set.
	if configDir := v.Env["XDG_CONFIG_HOME"]; configDir != "" {
		paths = append(paths, filepath.Join(configDir, "opentofu", "tofurc"))
	}

	return paths
}

// decodeOCITofuCredentials extracts the oci_credentials and
// oci_default_credentials blocks, ignoring the rest of the CLI config. An
// invalid block is an error, matching the diagnostics tofu reports.
func decodeOCITofuCredentials(data []byte, path string) (ociTofuCredentials, error) {
	file, diags := parseOCITofuConfig(data, path)
	if diags.HasErrors() {
		return ociTofuCredentials{}, diags
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "oci_credentials", LabelNames: []string{"repository_prefix"}},
			{Type: "oci_default_credentials"},
		},
	}

	content, _, diags := file.Body.PartialContent(schema)
	if diags.HasErrors() {
		return ociTofuCredentials{}, diags
	}

	tofu := ociTofuCredentials{
		repos:           make([]ociTofuRepoCredential, 0, len(content.Blocks)),
		discoverAmbient: true,
	}

	for _, block := range content.Blocks {
		if block.Type == "oci_default_credentials" {
			if tofu.hasDefault {
				return ociTofuCredentials{}, errOCIDuplicateDefaultBlock
			}

			defaults, err := decodeOCITofuDefaultHelper(block.Body)
			if err != nil {
				return ociTofuCredentials{}, err
			}

			tofu.hasDefault = true
			tofu.discoverAmbient = defaults.discoverAmbient
			tofu.defaultHelper = defaults.helper
			tofu.configFiles = defaults.configFiles
			tofu.configFilesSet = defaults.configFilesSet

			continue
		}

		repo, err := decodeOCITofuRepoBlock(block)
		if err != nil {
			return ociTofuCredentials{}, fmt.Errorf("oci_credentials %q: %w", block.Labels[0], err)
		}

		tofu.repos = append(tofu.repos, repo)
	}

	return tofu, nil
}

// decodeOCITofuDefaultHelper reads the fallback helper and whether ambient
// discovery is enabled (default true). Unknown arguments are tolerated so a
// newer tofu config still loads.
func decodeOCITofuDefaultHelper(body hcl.Body) (ociTofuDefaults, error) {
	var decoded struct {
		DiscoverAmbient *bool     `hcl:"discover_ambient_credentials,optional"`
		Helper          *string   `hcl:"docker_credentials_helper,optional"`
		ConfigFiles     *[]string `hcl:"docker_style_config_files,optional"`
		Remain          hcl.Body  `hcl:",remain"`
	}

	if diags := gohcl.DecodeBody(body, nil, &decoded); diags.HasErrors() {
		return ociTofuDefaults{}, diags
	}

	discoverAmbient := decoded.DiscoverAmbient == nil || *decoded.DiscoverAmbient

	if decoded.Helper != nil && !ociValidHelperName(*decoded.Helper) {
		return ociTofuDefaults{discoverAmbient: discoverAmbient}, errOCIInvalidHelperName
	}

	// The file list only tunes ambient discovery, so tofu rejects it when discovery is off.
	if !discoverAmbient && decoded.ConfigFiles != nil {
		return ociTofuDefaults{discoverAmbient: discoverAmbient}, errOCIAmbientFilesWithoutDiscovery
	}

	defaults := ociTofuDefaults{
		helper:          derefString(decoded.Helper),
		discoverAmbient: discoverAmbient,
	}

	// Absent keeps the default paths; any explicit list, even empty, replaces them.
	if decoded.ConfigFiles != nil {
		defaults.configFiles = *decoded.ConfigFiles
		defaults.configFilesSet = true
	}

	return defaults, nil
}

// decodeOCITofuRepoBlock reads one oci_credentials block, mapping OpenTofu's
// mutually-exclusive basic-auth, OAuth, and helper arguments. Unknown arguments
// are tolerated; configuring more than one style is rejected, matching tofu.
func decodeOCITofuRepoBlock(block *hcl.Block) (ociTofuRepoCredential, error) {
	var decoded struct {
		Username     *string  `hcl:"username,optional"`
		Password     *string  `hcl:"password,optional"`
		AccessToken  *string  `hcl:"access_token,optional"`
		RefreshToken *string  `hcl:"refresh_token,optional"`
		Helper       *string  `hcl:"docker_credentials_helper,optional"`
		Remain       hcl.Body `hcl:",remain"`
	}

	if diags := gohcl.DecodeBody(block.Body, nil, &decoded); diags.HasErrors() {
		return ociTofuRepoCredential{}, diags
	}

	// tofu picks the style from argument presence, so an empty value still selects it.
	basic := decoded.Username != nil || decoded.Password != nil
	oauth := decoded.AccessToken != nil || decoded.RefreshToken != nil
	helper := decoded.Helper != nil

	// Reject a username without a password, or the reverse, matching OpenTofu.
	if basic && (decoded.Username == nil || decoded.Password == nil) {
		return ociTofuRepoCredential{}, errOCIIncompleteBasicCredential
	}

	// OpenTofu requires the OAuth pair together, so reject a lone token.
	if oauth && (decoded.AccessToken == nil || decoded.RefreshToken == nil) {
		return ociTofuRepoCredential{}, errOCIIncompleteOAuthCredential
	}

	// tofu requires exactly one style; zero would shadow a valid ambient credential.
	switch trueCount(basic, oauth, helper) {
	case 1:
	case 0:
		return ociTofuRepoCredential{}, errOCIMissingCredentialStyle
	default:
		return ociTofuRepoCredential{}, errOCIMultipleCredentialStyles
	}

	if helper && !ociValidHelperName(*decoded.Helper) {
		return ociTofuRepoCredential{}, errOCIInvalidHelperName
	}

	// tofu parses the label as a bare repository address, so a URL scheme is invalid.
	if strings.Contains(block.Labels[0], "://") {
		return ociTofuRepoCredential{}, errOCILabelNotRepositoryAddress
	}

	registryDomain, repositoryPrefix := ociSplitRepositoryPrefix(block.Labels[0])

	// Docker credential helpers key on a whole domain, so tofu rejects a repository path here.
	if helper && repositoryPrefix != "" {
		return ociTofuRepoCredential{}, errOCIHelperWithRepositoryPath
	}

	repo := ociTofuRepoCredential{
		registryDomain:   ociCanonicalAuthKey(registryDomain),
		repositoryPrefix: repositoryPrefix,
		helper:           derefString(decoded.Helper),
	}

	if oauth {
		if *decoded.AccessToken == "" || *decoded.RefreshToken == "" {
			return ociTofuRepoCredential{}, errOCIEmptyCredentialValue
		}

		repo.cred = auth.Credential{
			AccessToken:  *decoded.AccessToken,
			RefreshToken: *decoded.RefreshToken,
		}

		return repo, nil
	}

	if basic {
		if *decoded.Username == "" || *decoded.Password == "" {
			return ociTofuRepoCredential{}, errOCIEmptyCredentialValue
		}

		repo.cred = auth.Credential{
			Username: *decoded.Username,
			Password: *decoded.Password,
		}
	}

	return repo, nil
}

// trueCount returns how many of the given booleans are true.
func trueCount(bools ...bool) int {
	count := 0

	for _, b := range bools {
		if b {
			count++
		}
	}

	return count
}

// ociSplitRepositoryPrefix splits a "registry[/repo/path]" label into its
// registry domain and repository-path prefix, stripping any URL scheme and
// trailing slash first so a "https://ghcr.io/acme" label still matches ghcr.io.
func ociSplitRepositoryPrefix(label string) (registryDomain, repositoryPrefix string) {
	cleaned := strings.TrimPrefix(label, "https://")
	cleaned = strings.TrimPrefix(cleaned, "http://")
	cleaned = strings.TrimRight(cleaned, "/")

	registryDomain, repositoryPrefix, _ = strings.Cut(cleaned, "/")

	return registryDomain, repositoryPrefix
}

// derefString returns the pointed-to string, or the empty string when nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
