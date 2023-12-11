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

	elementH1 elementName = iota
	elementH2

	frontmatterName frontmatterKey = iota
	frontmatterDescription
)

var (
	// `strings.EqualFold` is used (case insensitive) while comparing
	docFiles = []string{"README.md", "README.adoc"}

	frontmatterKeys = map[string]frontmatterKey{
		"name":        frontmatterName,
		"description": frontmatterDescription,
	}
)

type elementName byte
type frontmatterKey byte

type Doc struct {
	content string
	fileExt string

	elementCache     map[elementName]string
	elementRegs      map[elementName]*regexp.Regexp
	elementStripRegs []*regexp.Regexp

	frontmatterCache map[frontmatterKey]string
	frontmatterReg   *regexp.Regexp
}

func newDoc(content, fileExt string) *Doc {
	doc := &Doc{
		content: content,
		fileExt: fileExt,

		elementRegs:    make(map[elementName]*regexp.Regexp),
		frontmatterReg: regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
	}

	switch fileExt {
	case mdExt:
		doc.elementRegs[elementH1] = regexp.MustCompile(`(?m)(?:^|\n)\#{1}\s?([^#][\S\s]+?)\n\#`)
		doc.elementRegs[elementH2] = regexp.MustCompile(`(?m)(?:^|\n)\#{2}\s?([^#][\S\s]+?)\n\#`)
		doc.elementStripRegs = []*regexp.Regexp{
			// code
			regexp.MustCompile("`{3}" + `.*[\r\n]+`),
			regexp.MustCompile("`(.+?)`"),
			// html
			regexp.MustCompile("<(.*?)>"),
			// bold
			regexp.MustCompile(`\*\*([^*]+)\*\*`),
			regexp.MustCompile(`__([^_]+)__`),
			// italic
			regexp.MustCompile(`\*([^*]+)\*`),
			regexp.MustCompile(`_([^_]+)_`),
			// setext header
			regexp.MustCompile(`^[=\-]{2,}\s*$`),
			// foot note
			regexp.MustCompile(`\[\^.+?\](\: .*?$)?`),
			regexp.MustCompile(`\s{0,2}\[.*?\]: .*?$`),
			// image
			regexp.MustCompile(`\!\[(.*?)\]\s?[\[\(].*?[\]\)]`),
			// link
			regexp.MustCompile(`\[([\S\s]*?)\][\[\(].*?[\]\)]`),
			// blockquote
			regexp.MustCompile(`>\s*`),
			// ref link
			regexp.MustCompile(`^\s{1,2}\[(.*?)\]: (\S+)( ".*?")?\s*$`),
			// header
			regexp.MustCompile(`(?m)^\#{1,6}\s*([^#]+)\s*(\#{1,6})?$`),
			// horizontal rule
			regexp.MustCompile(`^[-\*_]{3,}\s*$`),
		}

	case adocExt:
		doc.elementRegs[elementH1] = regexp.MustCompile(`(?m)(?:^|\n)={1}\s?([^=][\S\s]+?)\n=`)
		doc.elementRegs[elementH2] = regexp.MustCompile(`(?m)(?:^|\n)={2}\s?([^=][\S\s]+?)\n=`)
		doc.elementStripRegs = []*regexp.Regexp{
			// html
			regexp.MustCompile("<(.*?)>"),
			// image
			regexp.MustCompile(`image:[^\]]+]`),
		}
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

func (doc *Doc) FrontmatterName() string {
	return doc.frontmatter(frontmatterName)
}

func (doc *Doc) FrontmatterDescription() string {
	return doc.frontmatter(frontmatterDescription)
}

func (doc *Doc) Name() string {
	lines := strings.Split(doc.element(elementH1), "\n")
	return strings.TrimSpace(lines[0])
}

func (doc *Doc) Description(maxLenght int) string {
	var strLines []string

	if lines := strings.Split(doc.element(elementH1), "\n"); len(lines) > 1 {
		strLines = append(strLines, lines[1:]...)
	}
	if lines := strings.Split(doc.element(elementH2), "\n"); len(lines) > 1 {
		strLines = append(strLines, lines[1:]...)
	}

	str := strings.Join(strLines, "\n")

	for _, sripReg := range doc.elementStripRegs {
		str = sripReg.ReplaceAllString(str, "$1")
	}

	// remove redundant spaces and new lines
	str = strings.Join(strings.Fields(str), " ")

	sentences := strings.Split(str, ".")

	var desc string
	for _, sentence := range sentences {
		sentence = sentence + "."
		if desc != "" && len(desc+sentence) > maxLenght {
			break
		}
		desc += sentence
	}

	return desc
}

func (doc *Doc) frontmatter(key frontmatterKey) string {
	if doc.frontmatterReg == nil {
		return ""
	}

	if doc.frontmatterCache == nil {
		doc.frontmatterCache = make(map[frontmatterKey]string)

		match := doc.frontmatterReg.FindStringSubmatch(doc.content)
		if len(match) == 0 {
			return ""
		}

		lines := strings.Split(match[1], "\n")

		for _, line := range lines {
			if parts := strings.Split(line, ":"); len(parts) > 1 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				val := strings.TrimSpace(parts[1])

				if key, ok := frontmatterKeys[key]; ok {
					doc.frontmatterCache[key] = val
				}
			}
		}
	}

	return doc.frontmatterCache[key]
}

func (doc *Doc) element(name elementName) string {
	if doc.elementRegs == nil {
		return ""
	}

	if doc.elementCache == nil {
		doc.elementCache = make(map[elementName]string)

		for name, elementReg := range doc.elementRegs {
			match := elementReg.FindStringSubmatch(doc.content)
			if len(match) == 0 {
				continue
			}
			val := match[1]

			doc.elementCache[name] = val
		}
	}

	return doc.elementCache[name]
}
