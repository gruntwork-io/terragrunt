package getter

import (
	"errors"
	iofs "io/fs"
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

// loadOCITofuCredentials reads and decodes the CLI config's OCI blocks. It is
// read-only and best-effort: a missing or unparsable file yields no credentials
// (with ambient discovery left enabled) rather than an error.
func loadOCITofuCredentials(l log.Logger, v *venv.Venv) ociTofuCredentials {
	empty := ociTofuCredentials{discoverAmbient: true}

	path := ociTofuConfigPath(v)
	if path == "" {
		return empty
	}

	if _, err := v.FS.Stat(path); err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			l.Warnf("Skipping unreadable OpenTofu CLI config %s: %v", path, err)
		}

		return empty
	}

	data, err := vfs.ReadFile(v.FS, path)
	if err != nil {
		l.Warnf("Skipping unreadable OpenTofu CLI config %s: %v", path, err)
		return empty
	}

	tofu, err := decodeOCITofuCredentials(l, data, path)
	if err != nil {
		l.Warnf("Skipping unparsable OpenTofu CLI config %s: %v", path, err)
		return empty
	}

	tofu.configDir = filepath.Dir(path)

	// Longest repository prefix first, so the most specific block wins.
	slices.SortStableFunc(tofu.repos, func(a, b ociTofuRepoCredential) int {
		return len(b.repositoryPrefix) - len(a.repositoryPrefix)
	})

	return tofu
}

// ociTofuConfigPath resolves the CLI config path OpenTofu would use: the
// TF_CLI_CONFIG_FILE or TERRAFORM_CONFIG override, else the first of
// ~/.tofurc, ~/.terraformrc that exists.
func ociTofuConfigPath(v *venv.Venv) string {
	if override := v.Env[envTFCLIConfigFile]; override != "" {
		return override
	}

	if override := v.Env[envTerraformConfig]; override != "" {
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

	// Windows keeps the CLI config in the roaming application-data directory.
	if v.Platform.GOOS == windowsGOOS {
		if appData := v.Env["APPDATA"]; appData != "" {
			paths = append(paths, filepath.Join(appData, "tofu.rc"), filepath.Join(appData, "terraform.rc"))
		}
	}

	if home != "" {
		paths = append(paths, filepath.Join(home, ".tofurc"), filepath.Join(home, ".terraformrc"))
	}

	if v.Platform.GOOS != windowsGOOS {
		configDir := v.Env["XDG_CONFIG_HOME"]
		if configDir == "" && home != "" {
			configDir = filepath.Join(home, ".config")
		}

		if configDir != "" {
			paths = append(paths, filepath.Join(configDir, "opentofu", "tofurc"))
		}
	}

	return slices.Compact(paths)
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

	if trueCount(basic, oauth, helper) > 1 {
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
