package module

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gitsight/go-vcsurl"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	"gopkg.in/ini.v1"
)

const (
	githubHost    = "github.com"
	gitlabHost    = "gitlab.com"
	azuredevHost  = "dev.azure.com"
	bitbucketHost = "bitbucket.org"

	tempDirFormat = "catalog-%x"
)

var (
	gitHeadBranchName = regexp.MustCompile(`^.*?([^/]+)$`)

	modulesPaths = []string{"modules"}
)

type Repo struct {
	cloneUrl string
	path     string

	remoteURL  string
	branchName string
}

func NewRepo(ctx context.Context, path string) (*Repo, error) {
	repo := &Repo{
		cloneUrl: path,
		path:     path,
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

		err := filepath.Walk(modulesPath,
			func(dir string, remote os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if !remote.IsDir() {
					return nil
				}

				moduleDir, err := filepath.Rel(repo.path, dir)
				if err != nil {
					return errors.WithStackTrace(err)
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

// moduleURL returns the URL of the module in this repository. `moduleDir` is the path from the repository root.
func (repo *Repo) moduleURL(moduleDir string) (string, error) {
	if repo.remoteURL == "" {
		return filepath.Join(repo.path, moduleDir), nil
	}

	remote, err := vcsurl.Parse(repo.remoteURL)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	switch remote.Host {
	case githubHost:
		return fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.branchName, moduleDir), nil
	case gitlabHost:
		return fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.branchName, moduleDir), nil
	case bitbucketHost:
		return fmt.Sprintf("https://%s/%s/browse/%s?at=%s", remote.Host, remote.FullName, moduleDir, repo.branchName), nil
	case azuredevHost:
		return fmt.Sprintf("https://%s/_git/%s?path=%s&version=GB%s", remote.Host, remote.FullName, moduleDir, repo.branchName), nil
	default:
		return "", errors.Errorf("hosting: %q is not supported yet", remote.Host)
	}
}

// clone clones the repository to a temporary directory if the repoPath is URL
func (repo *Repo) clone(ctx context.Context) error {
	if repo.path == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return errors.WithStackTrace(err)
		}

		repo.path = currentDir
	}

	if files.IsDir(repo.path) {
		if !filepath.IsAbs(repo.path) {
			absRepoPath, err := filepath.Abs(repo.path)
			if err != nil {
				return errors.WithStackTrace(err)
			}

			log.Debugf("Converting relative path %q to absolute %q", repo.path, absRepoPath)

			repo.path = absRepoPath
		}

		return nil
	}

	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf(tempDirFormat, util.EncodeBase64Sha1(repo.path)))

	if !files.FileExists(tempDir) {
		if err := os.Mkdir(tempDir, os.ModePerm); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	sourceUrl, err := terraform.ToSourceUrl(repo.cloneUrl, "")
	if err != nil {
		return err
	}

	// specify git:: scheme for the module URL
	if strings.HasPrefix(sourceUrl.Scheme, "http") {
		sourceUrl.Scheme = "git::" + sourceUrl.Scheme
	}
	repo.cloneUrl = sourceUrl.String()

	log.Infof("Cloning repository %q to temprory directory %q", repo.cloneUrl, tempDir)

	// if no repo directory is specified, `go-getter` returns the error "git exited with 128: fatal: not a git repository (or any of the parent directories"
	if !strings.Contains(sourceUrl.RequestURI(), "//") {
		sourceUrl.Path += "//."
	}

	if err := getter.GetAny(tempDir, strings.Trim(sourceUrl.String(), "/"), getter.WithContext(ctx)); err != nil {
		return errors.WithStackTrace(err)
	}

	repo.path = tempDir
	return nil
}

// parseRemoteURL reads the git config `.git/config` and parses the first URL of the remote URLs, the remote name "origin" has the highest priority.
func (repo *Repo) parseRemoteURL() error {
	gitConfigPath := filepath.Join(repo.path, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return errors.Errorf("the specified path %q is not a git repository", repo.path)
	}

	log.Debugf("Parsing git config %q", gitConfigPath)

	inidata, err := ini.Load(gitConfigPath)
	if err != nil {
		return errors.WithStackTrace(err)
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

	repo.remoteURL = inidata.Section(sectionName).Key("url").String()
	log.Debugf("Remote url: %q for repo: %q", repo.remoteURL, repo.path)

	return nil
}

// parseBranchName reads `.git/HEAD` file and parses a branch name.
func (repo *Repo) parseBranchName() error {
	gitHeadFile := filepath.Join(repo.path, ".git", "HEAD")

	data, err := files.ReadFileAsString(gitHeadFile)
	if err != nil {
		return errors.Errorf("the specified path %q is not a git repository", repo.path)
	}

	if match := gitHeadBranchName.FindStringSubmatch(data); len(match) > 0 {
		repo.branchName = strings.TrimSpace(match[1])
		return nil
	}

	return errors.Errorf("could not get branch name for repo %q", repo.path)
}
