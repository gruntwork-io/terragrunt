package module

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	mdHeader   = "#"
	adocHeader = "="
)

var (
	// `strings.EqualFold` is used (case insensitive) while comparing
	acceptableReadmeFiles = []string{"README.md", "README.adoc"}

	mdHeaderReg   = regexp.MustCompile(`(?m)^#{1}\s?([^#][\S\s]+)`)
	adocHeaderReg = regexp.MustCompile(`(?m)^={1}\s?([^=][\S\s]+)`)

	commentReg   = regexp.MustCompile(`<!--[\S\s]*?-->`)
	adocImageReg = regexp.MustCompile(`image:[^\]]+]`)

	terraformFileExts = []string{".tf"}
	ignoreFiles       = []string{"terraform-cloud-enterprise-private-module-registry-placeholder.tf"}

	defaultDescription = "(no description found)"
)

type Modules []*Module

type Module struct {
	repoPath    string
	moduleDir   string
	url         string
	title       string
	description string
	readme      string
}

// NewModule returns a module instance if the given `moduleDir` path contains a Terraform module, otherwise returns nil.
func NewModule(repo *Repo, moduleDir string) (*Module, error) {
	module := &Module{
		repoPath:    repo.path,
		moduleDir:   moduleDir,
		title:       filepath.Base(moduleDir),
		description: defaultDescription,
	}

	if ok, err := module.isValid(); !ok || err != nil {
		return nil, err
	}

	log.Debugf("Found module in directory %q", moduleDir)

	moduleURL, err := repo.moduleURL(moduleDir)
	if err != nil {
		return nil, err
	}
	module.url = moduleURL

	if err := module.parseReadme(); err != nil {
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
	return fmt.Sprintf("%s//%s", module.repoPath, module.moduleDir)

}

func (module *Module) isValid() (bool, error) {
	files, err := os.ReadDir(filepath.Join(module.repoPath, module.moduleDir))
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

func (module *Module) parseReadme() error {
	var readmePath string

	modulePath := filepath.Join(module.repoPath, module.moduleDir)

	files, err := os.ReadDir(modulePath)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		for _, readmeFile := range acceptableReadmeFiles {
			if strings.EqualFold(readmeFile, file.Name()) {
				readmePath = filepath.Join(modulePath, file.Name())
				break
			}
		}

		// `md` files have priority over `adoc` files
		if strings.EqualFold(filepath.Ext(readmePath), ".md") {
			break
		}
	}

	if readmePath == "" {
		return nil
	}

	readmeByte, err := os.ReadFile(readmePath)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	module.readme = string(readmeByte)

	var (
		reg       = mdHeaderReg
		docHeader = mdHeader
	)

	if strings.HasSuffix(readmePath, ".adoc") {
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
