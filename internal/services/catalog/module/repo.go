package module

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/util"

	"github.com/gitsight/go-vcsurl"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	gitpkg "github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter/v2"
	urlhelper "github.com/hashicorp/go-getter/v2/helper/url"
	"gopkg.in/ini.v1"
)

const (
	githubHost            = "github.com"
	githubEnterpriseRegex = `^(github\.(.+))$`
	gitlabHost            = "gitlab.com"
	azuredevHost          = "dev.azure.com"
	bitbucketHost         = "bitbucket.org"
	gitlabSelfHostedRegex = `^(gitlab\.(.+))$`

	cloneCompleteSentinel = ".catalog-clone-complete"
)

var (
	gitHeadBranchNameReg    = regexp.MustCompile(`^.*?([^/]+)$`)
	repoNameFromCloneURLReg = regexp.MustCompile(`(?i)^.*?([-a-z0-9_.]+)[^/]*?(?:\.git)?$`)

	modulesPaths = []string{"modules"}

	includedGitFiles = []string{"HEAD", "config"}
)

type Repo struct {
	Logger log.Logger

	cloneURL       string
	sourceURL      string
	path           string
	rootWorkingDir string

	RemoteURL  string
	BranchName string
	LatestTag  string

	walkWithSymlinks bool
	allowCAS         bool
	slowReporting    bool
}

// RepoOpts contains parameters for NewRepo.
type RepoOpts struct {
	CloneURL         string
	Path             string
	RootWorkingDir   string
	WalkWithSymlinks bool
	AllowCAS         bool
	SlowReporting    bool
}

func NewRepo(ctx context.Context, l log.Logger, opts RepoOpts) (*Repo, error) {
	repo := &Repo{
		Logger:           l,
		cloneURL:         opts.CloneURL,
		sourceURL:        opts.CloneURL,
		path:             opts.Path,
		walkWithSymlinks: opts.WalkWithSymlinks,
		allowCAS:         opts.AllowCAS,
		slowReporting:    opts.SlowReporting,
		rootWorkingDir:   opts.RootWorkingDir,
	}

	if err := repo.clone(ctx, l); err != nil {
		return nil, err
	}

	if err := repo.parseRemoteURL(); err != nil {
		return nil, err
	}

	if err := repo.parseBranchName(); err != nil {
		return nil, err
	}

	return repo, nil
}

// FindModules clones the repository if `repoPath` is a URL, searches for Terragrunt modules, indexes their README.* files, and returns module instances.
func (repo *Repo) FindModules(ctx context.Context) (Modules, error) {
	var modules Modules

	// check if root repo path is a module dir
	if module, err := NewModule(repo, ""); err != nil {
		return nil, err
	} else if module != nil {
		modules = append(modules, module)
	}

	for _, modulesPath := range modulesPaths {
		modulesPath = filepath.Join(repo.path, modulesPath)

		if !files.FileExists(modulesPath) {
			continue
		}

		walkFunc := filepath.WalkDir
		if repo.walkWithSymlinks {
			walkFunc = util.WalkDirWithSymlinks
		}

		err := walkFunc(modulesPath,
			func(dir string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				if !d.IsDir() {
					return nil
				}

				moduleDir, err := filepath.Rel(repo.path, dir)
				if err != nil {
					return errors.New(err)
				}

				moduleDir = filepath.ToSlash(moduleDir)

				if module, err := NewModule(repo, moduleDir); err != nil {
					return err
				} else if module != nil {
					modules = append(modules, module)
				}

				return nil
			})
		if err != nil {
			return nil, err
		}
	}

	return modules, nil
}

var githubEnterprisePatternReg = regexp.MustCompile(githubEnterpriseRegex)
var gitlabSelfHostedPatternReg = regexp.MustCompile(gitlabSelfHostedRegex)

// ModuleURL returns the URL to view this module in a browser.
// When the module provided is in a format that is not supported by the catalog, it returns an empty string.
func (repo *Repo) ModuleURL(moduleDir string) string {
	if repo.RemoteURL == "" {
		return filepath.Join(repo.path, moduleDir)
	}

	remote, err := vcsurl.Parse(repo.RemoteURL)
	if err != nil {
		return ""
	}

	// Simple, predictable hosts
	switch remote.Host {
	case githubHost:
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	case gitlabHost:
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	case bitbucketHost:
		return fmt.Sprintf("https://%s/%s/browse/%s?at=%s", remote.Host, remote.FullName, moduleDir, repo.BranchName)
	case azuredevHost:
		return fmt.Sprintf("https://%s/_git/%s?path=%s&version=GB%s", remote.Host, remote.FullName, moduleDir, repo.BranchName)
	}

	// // Hosts that require special handling
	if githubEnterprisePatternReg.MatchString(string(remote.Host)) {
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	}

	if gitlabSelfHostedPatternReg.MatchString(string(remote.Host)) {
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	}

	return ""
}

// SourceURL returns the original catalog URL before go-getter transformation.
func (repo *Repo) SourceURL() string {
	return repo.sourceURL
}

// Path returns the local filesystem path of the cloned (or local) repo. It
// may differ from the path originally passed via RepoOpts because the
// clone step can nest the working tree inside a repo-named subdirectory.
func (repo *Repo) Path() string {
	return repo.path
}

// CloneURL returns the resolved clone URL after go-getter normalization.
// This may differ from the URL originally passed via RepoOpts.
func (repo *Repo) CloneURL() string {
	return repo.cloneURL
}

// ResolveLatestTag looks up the latest semver release tag from the remote.
// The result is stored in LatestTag. If the lookup fails or the repo has no
// semver tags, LatestTag is left empty.
func (repo *Repo) ResolveLatestTag(ctx context.Context) {
	remote := repo.remoteForTagLookup()
	if remote == "" {
		return
	}

	runner, err := gitpkg.NewGitRunner(vexec.NewOSExec())
	if err != nil {
		repo.Logger.Debugf("catalog: skip tag lookup: %v", err)

		return
	}

	tag, err := runner.LatestReleaseTag(ctx, remote)
	if err != nil {
		repo.Logger.Debugf("catalog: failed to resolve latest tag for %q: %v", remote, err)

		return
	}

	repo.LatestTag = tag
}

type CloneOptions struct {
	Context    context.Context
	Logger     log.Logger
	SourceURL  string
	TargetPath string
}

func (repo *Repo) clone(ctx context.Context, l log.Logger) error {
	cloneURL := repo.resolveCloneURL()

	// Handle local directory case
	if files.IsDir(cloneURL) {
		return repo.handleLocalDir(cloneURL)
	}

	// Prepare clone options
	opts := CloneOptions{
		SourceURL:  cloneURL,
		TargetPath: repo.path,
		Context:    ctx,
		Logger:     repo.Logger,
	}

	if err := repo.prepareCloneDirectory(); err != nil {
		return err
	}

	if repo.cloneCompleted() {
		repo.Logger.Debugf("The repo dir exists and %q exists. Skipping cloning.", cloneCompleteSentinel)

		return nil
	}

	return repo.performClone(ctx, l, &opts)
}

func (repo *Repo) resolveCloneURL() string {
	if repo.cloneURL == "" {
		return repo.rootWorkingDir
	}

	return repo.cloneURL
}

func (repo *Repo) handleLocalDir(repoPath string) error {
	if !filepath.IsAbs(repoPath) {
		absRepoPath := filepath.Join(repo.rootWorkingDir, repoPath)
		repo.Logger.Debugf("Converting relative path %q to absolute %q", repoPath, absRepoPath)
		repo.path = absRepoPath

		return nil
	}

	repo.path = repoPath

	return nil
}

func (repo *Repo) prepareCloneDirectory() error {
	if err := os.MkdirAll(repo.path, os.ModePerm); err != nil {
		return errors.New(err)
	}

	repoName := repo.extractRepoName()
	repo.path = filepath.Join(repo.path, repoName)

	// Clean up incomplete clones
	if repo.shouldCleanupIncompleteClone() {
		repo.Logger.Debugf("The repo dir exists but %q does not. Removing the repo dir for cloning from the remote source.", cloneCompleteSentinel)

		if err := os.RemoveAll(repo.path); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func (repo *Repo) extractRepoName() string {
	repoName := "temp"
	if match := repoNameFromCloneURLReg.FindStringSubmatch(repo.cloneURL); len(match) > 0 && match[1] != "" {
		repoName = match[1]
	}

	return repoName
}

func (repo *Repo) shouldCleanupIncompleteClone() bool {
	return files.FileExists(repo.path) && !repo.cloneCompleted()
}

func (repo *Repo) cloneCompleted() bool {
	return files.FileExists(filepath.Join(repo.path, cloneCompleteSentinel))
}

func (repo *Repo) performClone(ctx context.Context, l log.Logger, opts *CloneOptions) error {
	client := getter.DefaultClient

	if repo.allowCAS {
		c, err := cas.New()
		if err != nil {
			return err
		}

		cloneOpts := cas.CloneOptions{
			Dir:              repo.path,
			IncludedGitFiles: includedGitFiles,
		}

		client.Getters = append([]getter.Getter{cas.NewCASGetter(l, c, &cloneOpts)}, client.Getters...)
	}

	sourceURL, err := tf.ToSourceURL(opts.SourceURL, "")
	if err != nil {
		return err
	}

	repo.cloneURL = sourceURL.String()
	opts.Logger.Infof("Cloning repository %q to temporary directory %q", repo.cloneURL, repo.path)

	// Check first if the query param ref is already set
	q := sourceURL.Query()

	ref := q.Get("ref")
	if ref == "" {
		q.Set("ref", "HEAD")
	}

	sourceURL.RawQuery = q.Encode()

	cloneFunc := func() error {
		_, err = client.Get(ctx, &getter.Request{
			Src:     sourceURL.String(),
			Dst:     repo.path,
			GetMode: getter.ModeDir,
		})

		return err
	}

	if repo.slowReporting {
		err = util.NotifyIfSlow(ctx, l, util.SpinnerWriter(), time.Second, util.SlowNotifyMsg{
			Spinner: "Cloning repository " + repo.cloneURL + "...",
			Done:    "Cloned repository " + repo.cloneURL,
		}, cloneFunc)
	} else {
		err = cloneFunc()
	}

	if err != nil {
		return err
	}

	// Create the sentinel file to indicate that the clone is complete
	f, err := os.Create(filepath.Join(repo.path, cloneCompleteSentinel))
	if err != nil {
		return errors.New(err)
	}

	if err := f.Close(); err != nil {
		return errors.New(err)
	}

	return nil
}

// parseRemoteURL reads the git config `.git/config` and parses the first URL of the remote URLs, the remote name "origin" has the highest priority.
func (repo *Repo) parseRemoteURL() error {
	gitConfigPath := filepath.Join(repo.path, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return errors.Errorf("the specified path %q is not a git repository (no .git/config file found)", repo.path)
	}

	repo.Logger.Debugf("Parsing git config %q", gitConfigPath)

	inidata, err := ini.Load(gitConfigPath)
	if err != nil {
		return errors.New(err)
	}

	var sectionName string

	for _, name := range inidata.SectionStrings() {
		if !strings.HasPrefix(name, "remote") {
			continue
		}

		sectionName = name

		if sectionName == `remote "origin"` {
			break
		}
	}

	// no git remotes found
	if sectionName == "" {
		return nil
	}

	repo.RemoteURL = inidata.Section(sectionName).Key("url").String()
	repo.Logger.Debugf("Remote url: %q for repo: %q", repo.RemoteURL, repo.path)

	return nil
}

func (repo *Repo) gitHeadfile() string {
	return filepath.Join(repo.path, ".git", "HEAD")
}

// parseBranchName reads `.git/HEAD` file and parses a branch name.
func (repo *Repo) parseBranchName() error {
	data, err := files.ReadFileAsString(repo.gitHeadfile())
	if err != nil {
		return errors.Errorf("the specified path %q is not a git repository (no .git/HEAD file found)", repo.path)
	}

	if match := gitHeadBranchNameReg.FindStringSubmatch(data); len(match) > 0 {
		repo.BranchName = strings.TrimSpace(match[1])

		return nil
	}

	return errors.Errorf("could not get branch name for repo %q", repo.path)
}

// remoteForTagLookup returns a URL suitable for git ls-remote.
// It prefers RemoteURL (parsed from .git/config) since that's what git
// originally used to clone. Falls back to cloneURL with go-getter
// prefixes, subdirectory paths, and query params stripped.
func (repo *Repo) remoteForTagLookup() string {
	if repo.RemoteURL != "" {
		return repo.RemoteURL
	}

	u := repo.cloneURL
	if u == "" {
		return ""
	}

	// Strip forced getter prefix (e.g. "git::", "s3::")
	if _, after, ok := strings.Cut(u, "::"); ok {
		u = after
	}

	// Strip //subdir suffix that go-getter uses to select a subdirectory.
	u, _ = getter.SourceDirSubdir(u)

	// Parse the URL so we can cleanly remove query parameters (e.g. "?ref=HEAD").
	parsed, err := urlhelper.Parse(u)
	if err != nil {
		return u
	}

	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}
