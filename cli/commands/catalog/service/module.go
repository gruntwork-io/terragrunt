package service

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"gopkg.in/ini.v1"
)

const (
	modulesPath = "/Users/levkohimins/Storage/work/contract/projects/terragrunt/repos/terraform-aws-eks"

	gitHubOrgURL = "https://github.com/gruntwork-io"

	mdHeader   = "#"
	adocHeader = "="
)

var (
	mdReg   = regexp.MustCompile(`(?m)^#{1}\s?([^#][\S\s]+)`)
	adocReg = regexp.MustCompile(`(?m)^={1}\s?([^=][\S\s]+)`)
)

type Modules []*Module

type Module struct {
	dir         string
	url         string
	title       string
	description string
	content     string
}

func (module *Module) Title() string {
	return module.title
}

func (module *Module) Description() string {
	return module.description
}

func (module *Module) Content() string {
	return module.content
}

func (module *Module) FilterValue() string {
	return module.title
}

func (module *Module) URL() string {
	return module.url
}

func (module *Module) Dir() string {
	return module.dir
}

func FindModules(rootPath string) (Modules, error) {
	var repoName string

	currentDir, err := os.Getwd()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if rootPath == "" {
		rootPath = currentDir
	}

	if files.IsDir(rootPath) {
		if !filepath.IsAbs(rootPath) {
			path, err := filepath.Abs(rootPath)
			if err != nil {
				return nil, errors.WithStackTrace(err)
			}

			rootPath = path
		}

		gitConfigPath := filepath.Join(rootPath, ".git", "config")

		if !files.FileExists(gitConfigPath) {
			return nil, errors.Errorf("git repository will not be found in the specified path %q", rootPath)
		}

		inidata, err := ini.Load(gitConfigPath)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}

		remoteURL := inidata.Section(`remote "origin"`).Key("url").String()
		if remoteURL == "" {
			return nil, errors.Errorf(`the specified git repository does not contain the remote "origin" URL`)
		}

		repoName = filepath.Base(remoteURL)
	}

	var modules Modules

	err = filepath.Walk(filepath.Join(rootPath, "modules"),
		func(dir string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return nil
			}

			if !files.FileExists(filepath.Join(dir, "main.tf")) || !files.FileExists(filepath.Join(dir, "variables.tf")) {
				return nil
			}

			var contentPath string

			for _, filename := range []string{"README.md", "README.adoc"} {
				path := filepath.Join(dir, filename)
				if files.FileExists(path) {
					contentPath = path
					break
				}
			}
			if contentPath == "" {
				return nil
			}

			contentByte, err := os.ReadFile(contentPath)
			if err != nil {
				return errors.WithStackTrace(err)
			}

			content := string(contentByte)

			moduleDir, err := filepath.Rel(rootPath, dir)
			if err != nil {
				return errors.WithStackTrace(err)
			}

			var (
				title            = moduleDir
				descriptionLines []string
			)

			reg := mdReg
			docHeader := mdHeader

			if strings.HasSuffix(contentPath, ".adoc") {
				reg = adocReg
				docHeader = adocHeader
			}

			match := reg.FindStringSubmatch(content)

			if len(match) > 0 {
				lines := strings.Split(match[1], "\n")
				title = strings.TrimSpace(lines[0])

				if len(lines) > 1 {
					for _, line := range lines[1:] {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "image:") || strings.HasPrefix(line, docHeader) {
							continue
						}

						descriptionLines = append(descriptionLines, line)
					}
				}
			}

			module := &Module{
				url:         path.Join(gitHubOrgURL, repoName, "tree/master", moduleDir),
				dir:         dir,
				title:       title,
				description: strings.TrimSpace(strings.Join(descriptionLines, " ")),
				content:     content,
			}

			modules = append(modules, module)

			return filepath.SkipDir
		})
	if err != nil {
		return nil, err
	}

	return modules, nil
}
