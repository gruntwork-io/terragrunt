package module

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gitsight/go-vcsurl"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/hashicorp/go-getter/v2"
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
	logger log.Logger

	cloneURL string
	path     string

	RemoteURL  string
	BranchName string

	walkWithSymlinks bool
	allowCAS         bool
}

func NewRepo(ctx context.Context, l log.Logger, cloneURL, path string, walkWithSymlinks bool, allowCAS bool) (*Repo, error) {
	repo := &Repo{
		logger:           l,
		cloneURL:         cloneURL,
		path:             path,
		walkWithSymlinks: walkWithSymlinks,
		allowCAS:         allowCAS,
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

type CloneOptions struct {
	Context    context.Context
	Logger     log.Logger
	SourceURL  string
	TargetPath string
}

func (repo *Repo) clone(ctx context.Context, l log.Logger) error {
	cloneURL, err := repo.resolveCloneURL()
	if err != nil {
		return err
	}

	// Handle local directory case
	if files.IsDir(cloneURL) {
		return repo.handleLocalDir(cloneURL)
	}

	// Prepare clone options
	opts := CloneOptions{
		SourceURL:  cloneURL,
		TargetPath: repo.path,
		Context:    ctx,
		Logger:     repo.logger,
	}

	if err := repo.prepareCloneDirectory(); err != nil {
		return err
	}

	if repo.cloneCompleted() {
		repo.logger.Debugf("The repo dir exists and %q exists. Skipping cloning.", cloneCompleteSentinel)

		return nil
	}

	return repo.performClone(ctx, l, &opts)
}

func (repo *Repo) resolveCloneURL() (string, error) {
	if repo.cloneURL == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", errors.New(err)
		}

		return currentDir, nil
	}

	return repo.cloneURL, nil
}

func (repo *Repo) handleLocalDir(repoPath string) error {
	if !filepath.IsAbs(repoPath) {
		absRepoPath, err := filepath.Abs(repoPath)
		if err != nil {
			return errors.New(err)
		}

		repo.logger.Debugf("Converting relative path %q to absolute %q", repoPath, absRepoPath)
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
		repo.logger.Debugf("The repo dir exists but %q does not. Removing the repo dir for cloning from the remote source.", cloneCompleteSentinel)

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
		c, err := cas.New(cas.Options{})
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

	_, err = client.Get(ctx, &getter.Request{
		Src:     sourceURL.String(),
		Dst:     repo.path,
		GetMode: getter.ModeDir,
	})
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

	repo.logger.Debugf("Parsing git config %q", gitConfigPath)

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
	repo.logger.Debugf("Remote url: %q for repo: %q", repo.RemoteURL, repo.path)

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
