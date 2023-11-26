package module

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/files"
)

const (
	modulesPath = "/Users/levkohimins/Storage/work/contract/projects/terragrunt/repos/terraform-aws-eks/modules"
)

var re = regexp.MustCompile(`(?m)^#{1}([^#]+)(.*)`)

func ScanModules() (Items, error) {
	var items Items

	err := filepath.Walk(modulesPath,
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

			var readmePath string

			markdownPath := filepath.Join(dir, "README.md")
			adocPath := filepath.Join(dir, "README.adoc")

			if files.FileExists(markdownPath) {
				readmePath = markdownPath
			}
			if files.FileExists(adocPath) {
				//readmePath = adocPath
			}

			if readmePath == "" {
				return nil
			}

			readmeByte, err := os.ReadFile(readmePath)
			if err != nil {
				panic(err)
			}

			readme := string(readmeByte)

			moduleDir := filepath.Base(dir)

			var (
				title       = moduleDir
				description string
			)

			match := re.FindStringSubmatch(readme)
			if len(match) > 0 {
				lines := strings.Split(match[1], "\n")

				title = strings.TrimSpace(lines[0])
				description = strings.TrimSpace(strings.Join(lines[1:], "\n"))
			}

			item := &Item{
				url:         path.Join("https://github.com/gruntwork-io", "terraform-aws-eks/tree/master/modules", moduleDir),
				dir:         dir,
				title:       title,
				description: description,
				readme:      readme,
			}

			items = append(items, item)

			return filepath.SkipDir
		})
	if err != nil {
		return nil, err
	}

	return items, nil
}
