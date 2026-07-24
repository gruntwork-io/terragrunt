package getter

import (
	"context"
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
	"oras.land/oras-go/v2/registry/remote/auth"
)

// Env vars OpenTofu reads to locate its CLI config, in the order it checks them.
const (
	envTFCLIConfigFile = "TF_CLI_CONFIG_FILE"
	envTerraformConfig = "TERRAFORM_CONFIG"
)

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

// ociTofuCredentials is the decoded OCI subset of an OpenTofu CLI config: its
// oci_credentials blocks, the oci_default_credentials fallback helper, and
// whether ambient discovery is enabled.
type ociTofuCredentials struct {
	defaultHelper   string
	repos           []ociTofuRepoCredential
	discoverAmbient bool
}

// repoCredential resolves the most specific oci_credentials block serving
// hostport/repositoryName. An empty result (no matching block, or a matching
// helper block with no entry) falls through to the caller's next tier.
func (c ociTofuCredentials) repoCredential(
	ctx context.Context,
	v venv.Venv,
	hostport, repositoryName string,
) (auth.Credential, error) {
	for i := range c.repos {
		repo := &c.repos[i]
		if !repo.matches(hostport, repositoryName) {
			continue
		}

		if repo.helper != "" {
			return ociCredentialFromHelper(ctx, v, ociHelperEntry{
				suffix:        repo.helper,
				serverAddress: ociTofuHelperServerAddress(hostport),
				explicit:      true,
			})
		}

		return repo.cred, nil
	}

	return auth.EmptyCredential, nil
}

// errOCIInvalidHelperName reports a credential helper name that is empty or
// contains a path separator, so it could execute a non-PATH binary.
var errOCIInvalidHelperName = errors.New("credential helper name must not be empty or contain a path separator")

// errOCIMultipleCredentialStyles reports an oci_credentials block configuring
// more than one of basic auth, OAuth, or a helper.
var errOCIMultipleCredentialStyles = errors.New("oci_credentials block must configure at most one credential style")

// errOCIIncompleteBasicCredential reports an oci_credentials block missing a username or password.
var errOCIIncompleteBasicCredential = errors.New("oci_credentials basic auth requires both a username and a password")

// ociValidHelperName reports whether name is a safe docker-credential suffix:
// non-empty and free of path separators, matching OpenTofu's validation.
func ociValidHelperName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`)
}

// loadOCITofuCredentials reads and decodes the CLI config's OCI blocks. It is
// read-only and best-effort: a missing or unparsable file yields no credentials
// (with ambient discovery left enabled) rather than an error.
func loadOCITofuCredentials(l log.Logger, v venv.Venv) ociTofuCredentials {
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

	// Longest repository prefix first, so the most specific block wins.
	slices.SortStableFunc(tofu.repos, func(a, b ociTofuRepoCredential) int {
		return len(b.repositoryPrefix) - len(a.repositoryPrefix)
	})

	return tofu
}

// ociTofuConfigPath resolves the CLI config path OpenTofu would use: the
// TF_CLI_CONFIG_FILE or TERRAFORM_CONFIG override, else the first of
// ~/.tofurc, ~/.terraformrc that exists.
func ociTofuConfigPath(v venv.Venv) string {
	if override := v.Env[envTFCLIConfigFile]; override != "" {
		return override
	}

	if override := v.Env[envTerraformConfig]; override != "" {
		return override
	}

	home, err := v.Platform.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	for _, name := range []string{".tofurc", ".terraformrc"} {
		candidate := filepath.Join(home, name)
		if _, statErr := v.FS.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	return ""
}

// decodeOCITofuCredentials extracts the oci_credentials and
// oci_default_credentials blocks, ignoring the rest of the CLI config. A single
// invalid block is skipped with a warning rather than discarding the whole file.
func decodeOCITofuCredentials(l log.Logger, data []byte, path string) (ociTofuCredentials, error) {
	file, diags := hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
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

			helper, discoverAmbient, err := decodeOCITofuDefaultHelper(block.Body)
			if err != nil {
				l.Warnf("Skipping invalid oci_default_credentials block in %s: %v", path, err)

				continue
			}

			tofu.defaultHelper = helper
			tofu.discoverAmbient = discoverAmbient

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
func decodeOCITofuDefaultHelper(body hcl.Body) (helper string, discoverAmbient bool, err error) {
	var decoded struct {
		DiscoverAmbient *bool    `hcl:"discover_ambient_credentials,optional"`
		Helper          *string  `hcl:"docker_credentials_helper,optional"`
		Remain          hcl.Body `hcl:",remain"`
		ConfigFiles     []string `hcl:"docker_style_config_files,optional"`
	}

	if diags := gohcl.DecodeBody(body, nil, &decoded); diags.HasErrors() {
		return "", true, diags
	}

	if decoded.Helper != nil && !ociValidHelperName(*decoded.Helper) {
		return "", true, errOCIInvalidHelperName
	}

	return derefString(decoded.Helper), decoded.DiscoverAmbient == nil || *decoded.DiscoverAmbient, nil
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
	oauth := decoded.AccessToken != nil || decoded.RefreshToken != nil
	helper := decoded.Helper != nil

	// Reject a username without a password, or the reverse, matching OpenTofu.
	if (decoded.Username != nil) != (decoded.Password != nil) {
		return ociTofuRepoCredential{}, errOCIIncompleteBasicCredential
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
