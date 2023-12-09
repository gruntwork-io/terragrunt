package module

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
)

const (
	mdExt   = ".md"
	adocExt = ".adoc"
)

var (
	// `strings.EqualFold` is used (case insensitive) while comparing
	docFiles = []string{"README.md", "README.adoc"}
)

type Doc struct {
	content string
	fileExt string

	h1Cache string
	h1Sign  string
	h1Reg   *regexp.Regexp

	imageReg   *regexp.Regexp
	commentReg *regexp.Regexp

	frontmatterCache map[string]string
	frontmatterReg   *regexp.Regexp
}

func newDoc(content, fileExt string) *Doc {
	doc := &Doc{
		content:        content,
		fileExt:        fileExt,
		commentReg:     regexp.MustCompile(`<!--[\S\s]*?-->`),
		frontmatterReg: regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
	}

	switch fileExt {
	case mdExt:
		doc.h1Sign = "#"
		doc.h1Reg = regexp.MustCompile(`(?m)^#{1}\s?([^#][\S\s]+)`)
	case adocExt:
		doc.h1Sign = "="
		doc.h1Reg = regexp.MustCompile(`(?m)^={1}\s?([^=][\S\s]+)`)
		doc.imageReg = regexp.MustCompile(`image:[^\]]+]`)
	}

	return doc
}

func FindDoc(dir string) (*Doc, error) {
	var filePath, fileExt string

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		for _, readmeFile := range docFiles {
			if strings.EqualFold(readmeFile, file.Name()) {
				filePath = filepath.Join(dir, file.Name())
				fileExt = filepath.Ext(filePath)
				break
			}
		}

		// `md` files have priority over `adoc` files
		if strings.EqualFold(fileExt, mdExt) {
			break
		}
	}

	if filePath == "" {
		return &Doc{}, nil
	}

	contentByte, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	content := string(contentByte)

	return newDoc(content, fileExt), nil
}

func (doc *Doc) frontmatter() map[string]string {
	if doc.frontmatterReg == nil || doc.frontmatterCache != nil {
		return doc.frontmatterCache
	}

	frontmatter := make(map[string]string)

	match := doc.frontmatterReg.FindStringSubmatch(doc.content)
	if len(match) == 0 {
		// to prevent running this reg again if nothing was found the first time
		doc.frontmatterReg = nil
		return frontmatter
	}
	lines := strings.Split(match[1], "\n")

	for _, line := range lines {
		if parts := strings.Split(line, ":"); len(parts) > 1 {
			frontmatter[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	doc.frontmatterCache = frontmatter
	return frontmatter
}

func (doc *Doc) h1() string {
	if doc.h1Reg == nil || doc.h1Cache != "" {
		return doc.h1Cache
	}

	match := doc.h1Reg.FindStringSubmatch(doc.content)
	if len(match) == 0 {
		// to prevent running this reg again if nothing was found the first time
		doc.h1Reg = nil
		return ""
	}
	header := match[1]

	if doc.commentReg != nil {
		// remove comments
		header = doc.commentReg.ReplaceAllString(header, "")
	}

	if doc.imageReg != nil {
		// remove images
		header = doc.imageReg.ReplaceAllString(header, "")
	}

	doc.h1Cache = header
	return header
}

func (doc *Doc) FrontmatterName() string {
	frontmatter := doc.frontmatter()
	return frontmatter["name"]
}

func (doc *Doc) FrontmatterDescription() string {
	frontmatter := doc.frontmatter()
	return frontmatter["description"]
}

func (doc *Doc) H1Title() string {
	lines := strings.Split(doc.h1(), "\n")

	return strings.TrimSpace(lines[0])
}

func (doc *Doc) H1Body() string {
	lines := strings.Split(doc.h1(), "\n")

	var descriptionLines []string

	if len(lines) > 1 {
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)

			// another header begins
			if strings.HasPrefix(line, doc.h1Sign) {
				break
			}

			descriptionLines = append(descriptionLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(descriptionLines, " "))
}
