package module

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	mdHeader   = "#"
	adocHeader = "="
)

var (
	readmeFiles = []string{"README.md", "README.adoc"}

	mdHeaderReg   = regexp.MustCompile(`(?m)^#{1}\s?([^#][\S\s]+)`)
	adocHeaderReg = regexp.MustCompile(`(?m)^={1}\s?([^=][\S\s]+)`)

	commentReg   = regexp.MustCompile(`<!--[\S\s]*?-->`)
	adocImageReg = regexp.MustCompile(`image:[^\]]+]`)

	terraformFileExts = []string{".tf"}
	ignoreFiles       = []string{"terraform-cloud-enterprise-private-module-registry-placeholder.tf"}
)

type Modules []*Module

type Module struct {
	path        string
	url         string
	title       string
	description string
	readme      string
}

// module returns a module instance if the given path `repoPath/moduleDir` contains a Terragrunt module.
func NewModule(repo *Repo, moduleDir string) (*Module, error) {
	module := &Module{
		path:  filepath.Join(repo.path, moduleDir),
		title: filepath.Base(moduleDir),
	}

	if ok, err := module.isValid(); !ok || err != nil {
		return nil, err
	}

	log.Debugf("Found module in directory %q", module.path)

	moduleURL, err := repo.moduleURL(moduleDir)
	if err != nil {
		return nil, err
	}
	module.url = moduleURL

	if err := module.parseDoc(); err != nil {
		return nil, err
	}

	return module, nil
}

// Title implements /github.com/charmbracelet/bubbles.list.DefaultItem.Title
func (module *Module) Title() string {
	return module.title
}

// Description implements /github.com/charmbracelet/bubbles.list.DefaultItem.Description
func (module *Module) Description() string {
	return module.description
}

func (module *Module) Readme() string {
	return module.readme
}

// FilterValue implements /github.com/charmbracelet/bubbles.list.Item.FilterValue
func (module *Module) FilterValue() string {
	return module.title
}

func (module *Module) URL() string {
	return module.url
}

func (module *Module) Path() string {
	return module.path
}

func (module *Module) isValid() (bool, error) {
	files, err := os.ReadDir(module.path)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if collections.ListContainsElement(ignoreFiles, file.Name()) {
			continue
		}

		ext := filepath.Ext(file.Name())
		if collections.ListContainsElement(terraformFileExts, ext) {
			return true, nil
		}
	}

	return false, nil
}

func (module *Module) parseDoc() error {
	var docPath string

	for _, filename := range readmeFiles {
		path := filepath.Join(module.path, filename)
		if files.FileExists(path) {
			docPath = path
			break
		}
	}

	if docPath == "" {
		return nil
	}

	docContentByte, err := os.ReadFile(docPath)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	module.readme = string(docContentByte)

	var (
		reg       = mdHeaderReg
		docHeader = mdHeader
	)

	if strings.HasSuffix(docPath, ".adoc") {
		reg = adocHeaderReg
		docHeader = adocHeader
	}

	if match := reg.FindStringSubmatch(module.readme); len(match) > 0 {
		header := match[1]

		// remove comments
		header = commentReg.ReplaceAllString(header, "")
		// remove adoc images
		header = adocImageReg.ReplaceAllString(header, "")

		lines := strings.Split(header, "\n")
		module.title = strings.TrimSpace(lines[0])

		var descriptionLines []string

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

		module.description = strings.TrimSpace(strings.Join(descriptionLines, " "))
	}

	return nil
}
