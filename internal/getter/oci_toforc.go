package getter

import (
	"errors"
	"path/filepath"
	"slices"
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

// ociValidHelperName reports whether name is a safe docker-credential suffix:
// non-empty and free of path separators, matching OpenTofu's validation.
func ociValidHelperName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`)
}

// loadOCITofuCredentials reads and merges the CLI config's OCI blocks. It is
// read-only and best-effort: a missing or unparsable source is skipped rather
// than failing the download.
func loadOCITofuCredentials(l log.Logger, v *venv.Venv) ociTofuCredentials {
	merged := ociTofuCredentials{discoverAmbient: true}
	seenRepos := make(map[string]struct{})

	for _, path := range ociTofuConfigSources(l, v) {
		data, err := vfs.ReadFile(v.FS, path)
		if err != nil {
			l.Warnf("Skipping unreadable OpenTofu CLI config %s: %v", path, err)

			continue
		}

		one, err := decodeOCITofuCredentials(l, data, path)
		if err != nil {
			l.Warnf("Skipping unparsable OpenTofu CLI config %s: %v", path, err)

			continue
		}

		mergeOCITofuCredentials(l, &merged, &one, path, seenRepos)
	}

	// Longest repository prefix first, so the most specific block wins.
	slices.SortStableFunc(merged.repos, func(a, b ociTofuRepoCredential) int {
		return len(b.repositoryPrefix) - len(a.repositoryPrefix)
	})

	return merged
}

// mergeOCITofuCredentials folds one decoded source into merged, keeping the
// first declaration of any repository label or default block, as OpenTofu
// rejects duplicates outright.
func mergeOCITofuCredentials(
	l log.Logger,
	merged, one *ociTofuCredentials,
	path string,
	seenRepos map[string]struct{},
) {
	for _, repo := range one.repos {
		key := repo.registryDomain + "/" + repo.repositoryPrefix
		if _, dup := seenRepos[key]; dup {
			l.Warnf("Ignoring duplicate oci_credentials block %q from %s", key, path)

			continue
		}

		seenRepos[key] = struct{}{}

		merged.repos = append(merged.repos, repo)
	}

	if !one.hasDefault {
		return
	}

	if merged.hasDefault {
		l.Warnf("Ignoring duplicate oci_default_credentials block in %s; at most one is allowed", path)

		return
	}

	merged.hasDefault = true
	merged.defaultHelper = one.defaultHelper
	merged.discoverAmbient = one.discoverAmbient
	merged.configFiles = one.configFiles
	merged.configFilesSet = one.configFilesSet
	// Relative docker_style_config_files resolve against the declaring file.
	merged.configDir = filepath.Dir(path)
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

	handle, err := v.FS.Open(dir)
	if err != nil {
		l.Warnf("Skipping unreadable OpenTofu CLI config directory %s: %v", dir, err)

		return nil
	}

	defer handle.Close() //nolint:errcheck

	names, err := handle.Readdirnames(-1)
	if err != nil {
		l.Warnf("Skipping unreadable OpenTofu CLI config directory %s: %v", dir, err)

		return nil
	}

	slices.Sort(names)

	var fragments []string

	for _, name := range names {
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
// JSON form OpenTofu also accepts (e.g. a *.tfrc.json named by TF_CLI_CONFIG_FILE).
func parseOCITofuConfig(data []byte, path string) (*hcl.File, hcl.Diagnostics) {
	if strings.HasSuffix(path, ".json") {
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

	// On Windows OpenTofu's home directory is the roaming application-data
	// folder, and it looks only for tofu.rc / terraform.rc there.
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

	// OpenTofu falls back to XDG only when XDG_CONFIG_HOME is set, so do not
	// synthesize a ~/.config path it would never read.
	if configDir := v.Env["XDG_CONFIG_HOME"]; configDir != "" {
		paths = append(paths, filepath.Join(configDir, "opentofu", "tofurc"))
	}

	return paths
}

// decodeOCITofuCredentials extracts the oci_credentials and
// oci_default_credentials blocks, ignoring the rest of the CLI config. A single
// invalid block is skipped with a warning rather than discarding the whole file.
func decodeOCITofuCredentials(l log.Logger, data []byte, path string) (ociTofuCredentials, error) {
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

	seenDefault := false

	for _, block := range content.Blocks {
		if block.Type == "oci_default_credentials" {
			if seenDefault {
				l.Warnf("Ignoring duplicate oci_default_credentials block in %s; at most one is allowed", path)

				continue
			}

			seenDefault = true

			defaults, err := decodeOCITofuDefaultHelper(block.Body)

			// Keep the discovery switch even when the rest of the block is
			// invalid, so a bad helper name cannot silently re-enable ambient.
			tofu.discoverAmbient = defaults.discoverAmbient

			if err != nil {
				l.Warnf("Skipping invalid oci_default_credentials block in %s: %v", path, err)

				continue
			}

			tofu.defaultHelper = defaults.helper
			tofu.configFiles = defaults.configFiles
			tofu.configFilesSet = defaults.configFilesSet
			tofu.hasDefault = true

			continue
		}

		repo, err := decodeOCITofuRepoBlock(block)
		if err != nil {
			l.Warnf("Skipping invalid oci_credentials block %q in %s: %v", block.Labels[0], path, err)

			continue
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
		return ociTofuDefaults{discoverAmbient: true}, diags
	}

	discoverAmbient := decoded.DiscoverAmbient == nil || *decoded.DiscoverAmbient

	if decoded.Helper != nil && !ociValidHelperName(*decoded.Helper) {
		return ociTofuDefaults{discoverAmbient: discoverAmbient}, errOCIInvalidHelperName
	}

	defaults := ociTofuDefaults{
		helper:          derefString(decoded.Helper),
		discoverAmbient: discoverAmbient,
	}

	// Absent means "use the default search paths"; an explicit list, even an
	// empty one, replaces them, so keep the two states apart.
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

	basic := decoded.Username != nil && decoded.Password != nil
	oauth := decoded.AccessToken != nil && decoded.RefreshToken != nil
	helper := decoded.Helper != nil

	// Reject a username without a password, or the reverse, matching OpenTofu.
	if (decoded.Username != nil) != (decoded.Password != nil) {
		return ociTofuRepoCredential{}, errOCIIncompleteBasicCredential
	}

	// OpenTofu requires the OAuth pair together, so reject a lone token.
	if (decoded.AccessToken != nil) != (decoded.RefreshToken != nil) {
		return ociTofuRepoCredential{}, errOCIIncompleteOAuthCredential
	}

	// OpenTofu requires exactly one style; zero would rank an empty credential
	// as a match and shadow a valid ambient one.
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

	registryDomain, repositoryPrefix := ociSplitRepositoryPrefix(block.Labels[0])

	repo := ociTofuRepoCredential{
		registryDomain:   ociCanonicalAuthKey(registryDomain),
		repositoryPrefix: repositoryPrefix,
		helper:           derefString(decoded.Helper),
	}

	if oauth {
		repo.cred = auth.Credential{
			AccessToken:  derefString(decoded.AccessToken),
			RefreshToken: derefString(decoded.RefreshToken),
		}

		return repo, nil
	}

	if basic {
		repo.cred = auth.Credential{
			Username: derefString(decoded.Username),
			Password: derefString(decoded.Password),
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

func derefString(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}
