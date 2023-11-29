package service

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/hashicorp/go-getter"
	"gopkg.in/ini.v1"
)

const (
	gitHubOrgURL = "https://github.com/gruntwork-io"

	mdHeader   = "#"
	adocHeader = "="
)

var (
	docFilenames = []string{"README.md", "README.adoc"}

	mdHeaderReg   = regexp.MustCompile(`(?m)^#{1}\s?([^#][\S\s]+)`)
	adocHeaderReg = regexp.MustCompile(`(?m)^={1}\s?([^=][\S\s]+)`)

	commentReg   = regexp.MustCompile(`<!--[\S\s]*?-->`)
	adocImageReg = regexp.MustCompile(`image:[^\]]+]`)
)

func getRepo(ctx context.Context, repoPath, tempDir string) (string, error) {
	if repoPath == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", errors.WithStackTrace(err)
		}

		repoPath = currentDir
	}

	if files.IsDir(repoPath) {
		if !filepath.IsAbs(repoPath) {
			repoPath, err := filepath.Abs(repoPath)
			if err != nil {
				return "", errors.WithStackTrace(err)
			}
			return repoPath, nil
		}

		return repoPath, nil
	}

	repoURL, err := terraform.ToSourceUrl(repoPath, tempDir)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	if !strings.Contains(repoURL.RequestURI(), "//") {
		repoURL.Path += "//."
	}

	if err := getter.GetAny(tempDir, repoURL.String(), getter.WithContext(ctx)); err != nil {
		return "", errors.WithStackTrace(err)
	}

	return tempDir, nil
}

func remoteURL(repoPath string) (string, error) {
	gitConfigPath := filepath.Join(repoPath, ".git", "config")

	if !files.FileExists(gitConfigPath) {
		return "", errors.Errorf("git repository will not be found in the specified path %q", repoPath)
	}

	inidata, err := ini.Load(gitConfigPath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	remoteURL := inidata.Section(`remote "origin"`).Key("url").String()
	if remoteURL == "" {
		return "", errors.Errorf(`the specified git repository does not contain the remote "origin" URL`)
	}

	return filepath.Base(remoteURL), nil
}

func moduleDocPath(modulePath string) string {
	if !files.FileExists(filepath.Join(modulePath, "main.tf")) || !files.FileExists(filepath.Join(modulePath, "variables.tf")) {
		return ""
	}

	for _, filename := range docFilenames {
		path := filepath.Join(modulePath, filename)
		if files.FileExists(path) {
			return path
		}
	}

	return ""
}

func module(repoName, repoPath, moduleDir string) (*Module, error) {
	var (
		modulePath = filepath.Join(repoPath, moduleDir)

		reg       = mdHeaderReg
		docHeader = mdHeader

		title            = filepath.Base(moduleDir)
		descriptionLines []string
	)

	docPath := moduleDocPath(modulePath)
	if docPath == "" {
		return nil, nil
	}

	docContentByte, err := os.ReadFile(docPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	docContent := string(docContentByte)

	if strings.HasSuffix(docPath, ".adoc") {
		reg = adocHeaderReg
		docHeader = adocHeader
	}

	if match := reg.FindStringSubmatch(docContent); len(match) > 0 {
		header := match[1]

		// remove comments
		header = commentReg.ReplaceAllString(header, "")
		// remove adoc images
		header = adocImageReg.ReplaceAllString(header, "")

		lines := strings.Split(header, "\n")
		title = strings.TrimSpace(lines[0])

		if len(lines) > 1 {
			for _, line := range lines[1:] {
				line = strings.TrimSpace(line)

				// another header begins
				if strings.HasPrefix(line, docHeader) {
					break
				}

				descriptionLines = append(descriptionLines, line)
			}
		}
	}

	return &Module{
		path:        modulePath,
		url:         path.Join(gitHubOrgURL, repoName, "tree/master", moduleDir),
		title:       title,
		description: strings.TrimSpace(strings.Join(descriptionLines, " ")),
		content:     docContent,
	}, nil

}

func FindModules(ctx context.Context, repoPath string) (Modules, error) {
	var repoName string

	tempDir, err := os.MkdirTemp("", "catalog-*")
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	defer os.RemoveAll(tempDir)

	repoPath, err = getRepo(ctx, repoPath, tempDir)
	if err != nil {
		return nil, err
	}

	remoteURL, err := remoteURL(repoPath)
	if err != nil {
		return nil, err
	}
	// remove extension like `.git`
	ext := filepath.Ext(remoteURL)
	remoteURL = strings.TrimRight(remoteURL, "."+ext)

	repoName = filepath.Base(remoteURL)

	modulesPath := filepath.Join(repoPath, "modules")

	// It is assumed that the repository is a module itself
	if !files.FileExists(modulesPath) {
		module, err := module(repoName, repoPath, "")
		if module == nil || err != nil {
			return nil, err
		}

		return Modules{module}, nil
	}

	var modules Modules

	err = filepath.Walk(modulesPath,
		func(dir string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return nil
			}

			moduleDir, err := filepath.Rel(repoPath, dir)
			if err != nil {
				return errors.WithStackTrace(err)
			}

			module, err := module(repoName, repoPath, moduleDir)
			if module == nil || err != nil {
				return err
			}
			modules = append(modules, module)

			return filepath.SkipDir
		})
	if err != nil {
		return nil, err
	}

	return modules, nil
}
