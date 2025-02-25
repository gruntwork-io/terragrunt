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
	"github.com/gruntwork-io/terragrunt/internal/cln"
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
	sshPartsLength        = 2
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
	useCln           bool
}

func NewRepo(ctx context.Context, logger log.Logger, cloneURL, tempDir string, walkWithSymlinks bool, useCln bool) (*Repo, error) {
	repo := &Repo{
		logger:           logger,
		cloneURL:         cloneURL,
		path:             tempDir,
		walkWithSymlinks: walkWithSymlinks,
		useCln:           useCln,
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
var gitlabSelfHostedPatternReg = regexp.MustCompile(gitlabSelfHostedRegex)

// ModuleURL returns the URL of the module in this repository. `moduleDir` is the path from the repository root.
func (repo *Repo) ModuleURL(moduleDir string) (string, error) {
	if repo.RemoteURL == "" {
		return filepath.Join(repo.path, moduleDir), nil
	}

	// If using cln:// protocol, strip it before parsing but preserve it for the final URL
	useClnProtocol := strings.HasPrefix(repo.RemoteURL, "cln://")
	remoteURL := repo.RemoteURL

	if useClnProtocol {
		remoteURL = strings.TrimPrefix(remoteURL, "cln://")
	}

	remote, err := vcsurl.Parse(remoteURL)
	if err != nil {
		return "", errors.New(err)
	}

	repo.logger.Infof("Generating URL for module %s in repository %s", moduleDir, repo.RemoteURL)

	// Simple, predictable hosts
	var url string

	switch {
	case remote.Host == githubHost:
		url = fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	case remote.Host == gitlabHost:
		url = fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
	case remote.Host == bitbucketHost:
		url = fmt.Sprintf("https://%s/%s/browse/%s?at=%s", remote.Host, remote.FullName, moduleDir, repo.BranchName)
	case remote.Host == azuredevHost:
		url = fmt.Sprintf("https://%s/_git/%s?path=%s&version=GB%s", remote.Host, remote.FullName, moduleDir, repo.BranchName)
	default:
		// Hosts that require special handling
		switch {
		case githubEnterprisePatternReg.MatchString(string(remote.Host)):
			url = fmt.Sprintf("https://%s/%s/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
		case gitlabSelfHostedPatternReg.MatchString(string(remote.Host)):
			url = fmt.Sprintf("https://%s/%s/-/tree/%s/%s", remote.Host, remote.FullName, repo.BranchName, moduleDir)
		default:
			return "", errors.Errorf("hosting: %q is not supported yet", remote.Host)
		}
	}

	// Add back cln:// protocol if it was present
	if useClnProtocol {
		url = "cln://" + url
	}

	return url, nil
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

	// Check for cln:// protocol
	isClnProtocol := strings.HasPrefix(repo.cloneURL, "cln://")
	if isClnProtocol && !repo.useCln {
		return errors.Errorf("the cln:// protocol requires the `cln` experiment to be enabled")
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

	// Check if URL uses cln:// protocol
	if isClnProtocol {
		// Strip the cln:// prefix and normalize the URL
		cloneURL := strings.TrimPrefix(repo.cloneURL, "cln://")

		// Handle SSH URLs (git@github.com:org/repo.git)
		if strings.HasPrefix(cloneURL, "git@") {
			// Normalize SSH URL by ensuring proper format
			parts := strings.Split(strings.TrimPrefix(cloneURL, "git@"), ":")
			if len(parts) != sshPartsLength {
				return errors.Errorf("invalid SSH URL format: %s", cloneURL)
			}

			host := parts[0]
			path := strings.TrimSuffix(parts[1], ".git")
			cloneURL = fmt.Sprintf("ssh://git@%s/%s.git", host, path)
			repo.logger.Debugf("Normalized SSH URL: %s", cloneURL)
		}

		// Remove existing directory if it exists
		if files.FileExists(repo.path) {
			if err := os.RemoveAll(repo.path); err != nil {
				return errors.New(err)
			}
		}

		cln, err := cln.New(cloneURL, cln.Options{
			Dir: repo.path,
		})
		if err != nil {
			return err
		}

		if err := cln.Clone(); err != nil {
			return errors.Errorf("failed to clone repository %q: %w", repo.cloneURL, err)
		}

		// For cln:// protocol, always use "main" as the branch name
		repo.BranchName = "main"

		return nil
	}

	// Non-cln protocol handling
	if files.FileExists(repo.path) && !files.FileExists(repo.gitHeadfile()) {
		repo.logger.Debugf("The repo dir exists but git file %q does not. Removing the repo dir for cloning from the remote source.", repo.gitHeadfile())

		if err := os.RemoveAll(repo.path); err != nil {
			return errors.New(err)
		}
	}

	sourceURL, err := tf.ToSourceURL(repo.cloneURL, "")
	if err != nil {
		return err
	}

	repo.cloneURL = sourceURL.String()
	repo.logger.Infof("Cloning repository %q to temporary directory %q", repo.cloneURL, repo.path)

	// Existing go-getter logic
	sourceURL.RawQuery = (url.Values{"ref": []string{"HEAD"}}).Encode()
	_, err = getter.Get(ctx, repo.path, strings.Trim(sourceURL.String(), "/"))

	return err
}

// parseRemoteURL reads the git config `.git/config` and parses the first URL of the remote URLs, the remote name "origin" has the highest priority.
func (repo *Repo) parseRemoteURL() error {
	// For cln repositories, use the original clone URL as the remote URL
	if strings.HasPrefix(repo.cloneURL, "cln://") {
		repo.RemoteURL = repo.cloneURL
		return nil
	}

	gitConfigPath := filepath.Join(repo.path, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return errors.Errorf("the specified path %q is not a git repository", repo.path)
	}

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

	return nil
}

func (repo *Repo) gitHeadfile() string {
	return filepath.Join(repo.path, ".git", "HEAD")
}

// parseBranchName reads `.git/HEAD` file and parses a branch name.
func (repo *Repo) parseBranchName() error {
	// If branch name is already set or using cln:// protocol, skip parsing
	if repo.BranchName != "" || strings.HasPrefix(repo.RemoteURL, "cln://") {
		return nil
	}

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
