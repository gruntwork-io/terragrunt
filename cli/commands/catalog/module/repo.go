package module

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gitsight/go-vcsurl"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/hashicorp/go-getter"
	"gopkg.in/ini.v1"
)

const (
	githubHost            = "github.com"
	githubEnterpriseRegex = `^(github\.(.+))$`
	gitlabHost            = "gitlab.com"
	azuredevHost          = "dev.azure.com"
	bitbucketHost         = "bitbucket.org"
)

var (
	gitHeadBranchNameReg    = regexp.MustCompile(`^.*?([^/]+)$`)
	repoNameFromCloneURLReg = regexp.MustCompile(`(?i)^.*?([-a-z_.]+)[^/]*?(?:\.git)?$`)

	modulesPaths = []string{"modules"}
)

type Repo struct {
	logger log.Logger

	cloneURL string
	path     string

	RemoteURL  string
	BranchName string

	walkWithSymlinks bool
}

func NewRepo(ctx context.Context, logger log.Logger, cloneURL, tempDir string, walkWithSymlinks bool) (*Repo, error) {
	repo := &Repo{
		logger:           logger,
		cloneURL:         cloneURL,
		path:             tempDir,
		walkWithSymlinks: walkWithSymlinks,
	}

	if err := repo.clone(ctx); err != nil {
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

		walkFunc := filepath.Walk
		if repo.walkWithSymlinks {
			walkFunc = util.WalkWithSymlinks
		}

		err := walkFunc(modulesPath,
			func(dir string, remote os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !remote.IsDir() {
					return nil
				}

				moduleDir, err := filepath.Rel(repo.path, dir)
				if err != nil {
					return errors.New(err)
				}

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

// ModuleURL returns the URL of the module in this repository. `moduleDir` is the path from the repository root.
func (repo *Repo) ModuleURL(moduleDir string) (string, error) {
	if repo.RemoteURL == "" {
		return filepath.Join(repo.path, moduleDir), nil
	}

	remote, err := vcsurl.Parse(repo.RemoteURL)
	if err != nil {
		return "", errors.New(err)
	}

	// Simple, predictable hosts
	switch remote.Host {
	case githubHost:
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	case gitlabHost:
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	case bitbucketHost:
		return fmt.Sprintf("https://%s/%s/browse/%s?at=%s", remote.Host, remote.FullName, moduleDir, repo.BranchName), nil
	case azuredevHost:
		return fmt.Sprintf("https://%s/_git/%s?path=%s&version=GB%s", remote.Host, remote.FullName, moduleDir, repo.BranchName), nil
	}

	// // Hosts that require special handling
	if githubEnterprisePatternReg.MatchString(string(remote.Host)) {
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir), nil
	}

	return "", errors.Errorf("hosting: %q is not supported yet", remote.Host)
}

// clone clones the repository to a temporary directory if the repoPath is URL
func (repo *Repo) clone(ctx context.Context) error {
	if repo.cloneURL == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.New(err)
		}

		repo.cloneURL = currentDir
	}

	if repoPath := repo.cloneURL; files.IsDir(repoPath) {
		if !filepath.IsAbs(repoPath) {
			absRepoPath, err := filepath.Abs(repoPath)
			if err != nil {
				return errors.New(err)
			}

			repo.logger.Debugf("Converting relative path %q to absolute %q", repoPath, absRepoPath)
		}

		repo.path = repoPath

		return nil
	}

	if err := os.MkdirAll(repo.path, os.ModePerm); err != nil {
		return errors.New(err)
	}

	repoName := "temp"
	if match := repoNameFromCloneURLReg.FindStringSubmatch(repo.cloneURL); len(match) > 0 && match[1] != "" {
		repoName = match[1]
	}

	repo.path = filepath.Join(repo.path, repoName)

	// Since we are cloning the repository into a temporary directory, some operating systems such as MacOS have a service for deleting files that have not been accessed for a long time.
	// For example, in MacOS the service is responsible for deleting unused files deletes only files while leaving the directory structure is untouched, which in turn misleads `go-getter`, which thinks that the repository exists but cannot update it due to the lack of files. In such cases, we simply delete the temporary directory in order to clone the one again.
	// See https://github.com/gruntwork-io/terragrunt/pull/2888
	if files.FileExists(repo.path) && !files.FileExists(repo.gitHeadfile()) {
		repo.logger.Debugf("The repo dir exists but git file %q does not. Removing the repo dir for cloning from the remote source.", repo.gitHeadfile())

		if err := os.RemoveAll(repo.path); err != nil {
			return errors.New(err)
		}
	}

	sourceURL, err := terraform.ToSourceURL(repo.cloneURL, "")
	if err != nil {
		return err
	}

	repo.cloneURL = sourceURL.String()

	repo.logger.Infof("Cloning repository %q to temporary directory %q", repo.cloneURL, repo.path)

	// We need to explicitly specify the reference, otherwise we will get an error:
	// "fatal: The empty string is not a valid pathspec. Use . instead if you wanted to match all paths"
	// when updating an existing repository.
	sourceURL.RawQuery = (url.Values{"ref": []string{"HEAD"}}).Encode()

	if err := getter.Get(repo.path, strings.Trim(sourceURL.String(), "/"), getter.WithContext(ctx), getter.WithMode(getter.ClientModeDir)); err != nil {
		return errors.New(err)
	}

	return nil
}

// parseRemoteURL reads the git config `.git/config` and parses the first URL of the remote URLs, the remote name "origin" has the highest priority.
func (repo *Repo) parseRemoteURL() error {
	gitConfigPath := filepath.Join(repo.path, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return errors.Errorf("the specified path %q is not a git repository", repo.path)
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
		return errors.Errorf("the specified path %q is not a git repository", repo.path)
	}

	if match := gitHeadBranchNameReg.FindStringSubmatch(data); len(match) > 0 {
		repo.BranchName = strings.TrimSpace(match[1])
		return nil
	}

	return errors.Errorf("could not get branch name for repo %q", repo.path)
}
