package getter

import (
	"bytes"
	"errors"
	"fmt"
	iofs "io/fs"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// ociTofuRepoCredential is one decoded oci_credentials block: registry, repository prefix, credential or helper.
type ociTofuRepoCredential struct {
	label            string
	registryDomain   string
	repositoryPrefix string
	cred             auth.Credential
	helper           string
}

// matches reports whether this block serves hostport/repositoryName; the trailing "/" path.Join strips is the boundary.
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

// specificity ranks this block by counting the repository-path segments the label pins, not by joining a path.
func (c *ociTofuRepoCredential) specificity() int {
	if c.repositoryPrefix == "" {
		return ociDomainSpecificity
	}

	return ociDomainSpecificity + strings.Count(c.repositoryPrefix, "/") + 1
}

// dedupKey identifies the label a block claims; concatenation keeps it injective, which path cleaning would not.
func (c *ociTofuRepoCredential) dedupKey() string {
	return c.registryDomain + "/" + c.repositoryPrefix
}

// ociTofuCredentials is the decoded OCI subset of an OpenTofu CLI config.
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

// ociTofuRepoBody is the decoded argument set of one oci_credentials block.
type ociTofuRepoBody struct {
	Username     *string  `hcl:"username,optional"`
	Password     *string  `hcl:"password,optional"`
	AccessToken  *string  `hcl:"access_token,optional"`
	RefreshToken *string  `hcl:"refresh_token,optional"`
	Helper       *string  `hcl:"docker_credentials_helper,optional"`
	Remain       hcl.Body `hcl:",remain"`
}

// ociValidHelperName reports whether name is a docker-credential-<name> suffix run off PATH, never a path itself.
func ociValidHelperName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`)
}

// ociTofuConfigError attributes one problem to its CLI config file, so every joined line locates itself.
func ociTofuConfigError(path string, err error) error {
	return fmt.Errorf("reading OpenTofu CLI config %s: %w", path, err)
}

// loadOCITofuCredentials merges the CLI config's OCI blocks; a file that exists but is invalid is an error.
func loadOCITofuCredentials(l log.Logger, v *venv.Venv) (ociTofuCredentials, error) {
	merged := ociTofuCredentials{discoverAmbient: true}
	seenRepos := make(map[string]struct{})

	sources, err := ociTofuConfigSources(l, v)
	if err != nil {
		return ociTofuCredentials{}, err
	}

	var errs []error

	for _, path := range sources {
		data, err := vfs.ReadFile(v.FS, path)
		if err != nil {
			errs = append(errs, ociTofuConfigError(path, err))

			continue
		}

		one, err := decodeOCITofuCredentials(data, path)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if err := mergeOCITofuCredentials(&merged, &one, path, seenRepos); err != nil {
			errs = append(errs, err)
		}
	}

	// Every source is reported at once, and no partial merge escapes alongside an error.
	if err := errors.Join(errs...); err != nil {
		return ociTofuCredentials{}, err
	}

	return merged, nil
}

// mergeOCITofuCredentials folds one decoded source into merged, rejecting duplicates as tofu does.
func mergeOCITofuCredentials(
	merged, one *ociTofuCredentials,
	path string,
	seenRepos map[string]struct{},
) error {
	var errs []error

	for i := range one.repos {
		repo := &one.repos[i]

		key := repo.dedupKey()
		if _, dup := seenRepos[key]; dup {
			errs = append(errs, OCIDuplicateRepoBlockError{Label: repo.label, Path: path})

			continue
		}

		seenRepos[key] = struct{}{}

		merged.repos = append(merged.repos, *repo)
	}

	if one.hasDefault {
		if merged.hasDefault {
			errs = append(errs, OCIDuplicateDefaultBlockError{Path: path})
		} else {
			merged.hasDefault = true
			merged.defaultHelper = one.defaultHelper
			merged.discoverAmbient = one.discoverAmbient
			merged.configFiles = one.configFiles
			merged.configFilesSet = one.configFilesSet
			// Relative docker_style_config_files resolve against the declaring file.
			merged.configDir = filepath.Dir(path)
		}
	}

	return errors.Join(errs...)
}

// ociTofuConfigSources lists the CLI config files to read; an env override suppresses the fragments.
func ociTofuConfigSources(l log.Logger, v *venv.Venv) ([]string, error) {
	path, err := ociTofuConfigPath(l, v)
	if err != nil {
		return nil, err
	}

	var sources []string

	if path != "" {
		sources = append(sources, path)
	}

	if override, _ := cliconfig.UserConfigOverride(v); override != "" {
		return sources, nil
	}

	fragments, err := ociTofuConfigFragments(l, v)
	if err != nil {
		return nil, err
	}

	return append(sources, fragments...), nil
}

// ociTofuConfigFragments returns the config directory's *.tfrc and *.tfrc.json files, in filename order.
func ociTofuConfigFragments(l log.Logger, v *venv.Venv) ([]string, error) {
	dir, err := cliconfig.UserConfigDir(v)
	if err != nil {
		return nil, err
	}

	if dir == "" {
		return nil, nil
	}

	// tofu reads no fragments when the config directory is absent, unreadable, or not a directory.
	info, err := v.FS.Stat(dir)
	if err != nil {
		if !errors.Is(err, iofs.ErrNotExist) {
			l.Warnf("Skipping unreadable OpenTofu CLI config directory %s: %v", dir, err)
		}

		return nil, nil
	}

	if !info.IsDir() {
		return nil, nil
	}

	entries, err := vfs.ReadDirEntries(v.FS, dir)
	if err != nil {
		l.Warnf("Skipping unreadable OpenTofu CLI config directory %s: %v", dir, err)

		return nil, nil
	}

	fragments := make([]string, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".tfrc") && !strings.HasSuffix(name, ".tfrc.json") {
			continue
		}

		fragments = append(fragments, filepath.Join(dir, name))
	}

	return fragments, nil
}

// ociTofuConfigPath resolves the CLI config path OpenTofu would use, honoring its env overrides.
func ociTofuConfigPath(l log.Logger, v *venv.Venv) (string, error) {
	if override, envName := cliconfig.UserConfigOverride(v); override != "" {
		exists, err := vfs.FileExists(v.FS, override)
		if err != nil {
			return "", ociTofuConfigError(override, err)
		}

		// tofu skips a named config file that is absent, so warn rather than fail, and read no other file.
		if !exists {
			l.Warnf("OpenTofu CLI config %s named by %s does not exist", override, envName)

			return "", nil
		}

		return override, nil
	}

	for _, candidate := range cliconfig.UserConfigCandidates(v) {
		exists, err := vfs.FileExists(v.FS, candidate)
		if err != nil {
			return "", ociTofuConfigError(candidate, err)
		}

		if exists {
			return candidate, nil
		}
	}

	return "", nil
}

// parseOCITofuConfig parses the CLI config, detecting the JSON form from a leading brace as tofu does.
func parseOCITofuConfig(data []byte, path string) (*hcl.File, hcl.Diagnostics) {
	if strings.HasSuffix(path, ".json") || bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return hcljson.Parse(data, path)
	}

	return hclsyntax.ParseConfig(data, path, hcl.Pos{Line: 1, Column: 1})
}

// decodeOCITofuCredentials extracts the OCI blocks; an invalid block is an error, as tofu reports.
func decodeOCITofuCredentials(data []byte, path string) (ociTofuCredentials, error) {
	file, diags := parseOCITofuConfig(data, path)
	if diags.HasErrors() {
		return ociTofuCredentials{}, ociTofuConfigError(path, diags)
	}

	schema := &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "oci_credentials", LabelNames: []string{"repository_prefix"}},
			{Type: "oci_default_credentials"},
		},
	}

	content, _, diags := file.Body.PartialContent(schema)
	if diags.HasErrors() {
		return ociTofuCredentials{}, ociTofuConfigError(path, diags)
	}

	tofu := ociTofuCredentials{
		repos:           make([]ociTofuRepoCredential, 0, len(content.Blocks)),
		discoverAmbient: true,
	}

	var errs []error

	for _, block := range content.Blocks {
		if block.Type == "oci_default_credentials" {
			if err := decodeOCITofuDefaultBlock(&tofu, block, path); err != nil {
				errs = append(errs, err)
			}

			continue
		}

		repo, err := decodeOCITofuRepoBlock(block)
		if err != nil {
			errs = append(errs, ociTofuConfigError(path, fmt.Errorf("oci_credentials %q: %w", block.Labels[0], err)))

			continue
		}

		tofu.repos = append(tofu.repos, repo)
	}

	// Every bad block is reported at once, so a user fixes the whole file in one pass.
	if err := errors.Join(errs...); err != nil {
		return ociTofuCredentials{}, err
	}

	return tofu, nil
}

// decodeOCITofuDefaultBlock folds one oci_default_credentials block into tofu, rejecting a second one.
func decodeOCITofuDefaultBlock(tofu *ociTofuCredentials, block *hcl.Block, path string) error {
	if tofu.hasDefault {
		return OCIDuplicateDefaultBlockError{Path: path}
	}

	defaults, err := decodeOCITofuDefaultHelper(block.Body)
	if err != nil {
		return ociTofuConfigError(path, err)
	}

	tofu.hasDefault = true
	tofu.discoverAmbient = defaults.discoverAmbient
	tofu.defaultHelper = defaults.helper
	tofu.configFiles = defaults.configFiles
	tofu.configFilesSet = defaults.configFilesSet

	return nil
}

// decodeOCITofuDefaultHelper reads the fallback helper and the ambient-discovery switch, default true.
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
		return ociTofuDefaults{discoverAmbient: discoverAmbient}, ErrOCIInvalidHelperName
	}

	// The file list only tunes ambient discovery, so tofu rejects it when discovery is off.
	if !discoverAmbient && decoded.ConfigFiles != nil {
		return ociTofuDefaults{discoverAmbient: discoverAmbient}, ErrOCIAmbientFilesWithoutDiscovery
	}

	defaults := ociTofuDefaults{
		helper:          util.Deref(decoded.Helper),
		discoverAmbient: discoverAmbient,
	}

	// Absent keeps the default paths; any explicit list, even empty, replaces them.
	if decoded.ConfigFiles != nil {
		defaults.configFiles = *decoded.ConfigFiles
		defaults.configFilesSet = true
	}

	return defaults, nil
}

// decodeOCITofuRepoBlock reads one oci_credentials block; a rejected block yields no credential, never a partial one.
func decodeOCITofuRepoBlock(block *hcl.Block) (ociTofuRepoCredential, error) {
	var decoded ociTofuRepoBody

	if diags := gohcl.DecodeBody(block.Body, nil, &decoded); diags.HasErrors() {
		return ociTofuRepoCredential{}, diags
	}

	// tofu picks the style from argument presence, so an empty value still selects it.
	basic := decoded.Username != nil || decoded.Password != nil
	oauth := decoded.AccessToken != nil || decoded.RefreshToken != nil
	helper := decoded.Helper != nil

	if err := validateOCITofuRepoStyle(&decoded, basic, oauth, helper); err != nil {
		return ociTofuRepoCredential{}, err
	}

	// tofu parses the label as a bare repository address, so a URL scheme is invalid.
	if strings.Contains(block.Labels[0], "://") {
		return ociTofuRepoCredential{}, ErrOCILabelNotRepositoryAddress
	}

	registryDomain, repositoryPrefix := ociSplitRepositoryPrefix(block.Labels[0])

	// Docker credential helpers key on a whole domain, so tofu rejects a repository path here.
	if helper && repositoryPrefix != "" {
		return ociTofuRepoCredential{}, ErrOCIHelperWithRepositoryPath
	}

	repo := ociTofuRepoCredential{
		label:            block.Labels[0],
		registryDomain:   ociCanonicalAuthKey(registryDomain),
		repositoryPrefix: repositoryPrefix,
		helper:           util.Deref(decoded.Helper),
	}

	// A helper block carries no inline secret, so it is complete once the helper name is validated.
	if !oauth && !basic {
		return repo, nil
	}

	cred, err := ociTofuInlineCredential(&decoded, oauth)
	if err != nil {
		return ociTofuRepoCredential{}, err
	}

	repo.cred = cred

	return repo, nil
}

// validateOCITofuRepoStyle enforces tofu's rule of exactly one complete credential style per block.
func validateOCITofuRepoStyle(decoded *ociTofuRepoBody, basic, oauth, helper bool) error {
	// Reject a username without a password, or the reverse, matching OpenTofu.
	if basic && (decoded.Username == nil || decoded.Password == nil) {
		return ErrOCIIncompleteBasicCredential
	}

	// OpenTofu requires the OAuth pair together, so reject a lone token.
	if oauth && (decoded.AccessToken == nil || decoded.RefreshToken == nil) {
		return ErrOCIIncompleteOAuthCredential
	}

	// tofu requires exactly one style; zero would shadow a valid ambient credential.
	switch trueCount(basic, oauth, helper) {
	case 1:
	case 0:
		return ErrOCIMissingCredentialStyle
	default:
		return ErrOCIMultipleCredentialStyles
	}

	if helper && !ociValidHelperName(*decoded.Helper) {
		return ErrOCIInvalidHelperName
	}

	return nil
}

// ociTofuInlineCredential builds the credential for whichever inline style the block selected.
func ociTofuInlineCredential(decoded *ociTofuRepoBody, oauth bool) (auth.Credential, error) {
	if oauth {
		return ociTofuOAuthCredential(*decoded.AccessToken, *decoded.RefreshToken)
	}

	return ociTofuBasicCredential(*decoded.Username, *decoded.Password)
}

// ociTofuOAuthCredential builds the OAuth pair, rejecting an empty token as tofu does.
func ociTofuOAuthCredential(accessToken, refreshToken string) (auth.Credential, error) {
	if accessToken == "" || refreshToken == "" {
		return auth.EmptyCredential, ErrOCIEmptyCredentialValue
	}

	return auth.Credential{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}

// ociTofuBasicCredential builds the basic-auth pair, rejecting an empty half as tofu does.
func ociTofuBasicCredential(username, password string) (auth.Credential, error) {
	if username == "" || password == "" {
		return auth.EmptyCredential, ErrOCIEmptyCredentialValue
	}

	return auth.Credential{Username: username, Password: password}, nil
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

// ociSplitRepositoryPrefix splits a registry[/repo/path] label into registry domain and repository prefix.
func ociSplitRepositoryPrefix(label string) (registryDomain, repositoryPrefix string) {
	registryDomain, repositoryPrefix, _ = strings.Cut(strings.TrimRight(label, "/"), "/")

	return registryDomain, repositoryPrefix
}
