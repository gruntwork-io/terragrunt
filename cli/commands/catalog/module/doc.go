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

	docTitle docDataKey = iota
	docDescription
	docContent

	elementH1Block docElementName = iota
	elementH2Block
)

var (
	// `strings.EqualFold` is used (case insensitive) while comparing
	docFiles = []string{"README.md", "README.adoc"}

	frontmatterKeys = map[string]docDataKey{
		"name":        docTitle,
		"description": docDescription,
	}
)

type docDataKey byte
type docElementName byte

type DocRegs []*regexp.Regexp

func (regs DocRegs) Replace(str string) string {
	for _, reg := range regs {
		str = reg.ReplaceAllString(str, "$1")
	}
	return str
}

type Doc struct {
	rawContent string
	fileExt    string

	elementCache     map[docDataKey]string
	elementRegs      map[docElementName]*regexp.Regexp
	elementStripRegs DocRegs

	frontmatterCache map[docDataKey]string
	frontmatterReg   *regexp.Regexp
}

func newDoc(rawContent, fileExt string) *Doc {
	doc := &Doc{
		rawContent: rawContent,
		fileExt:    fileExt,

		elementRegs:    make(map[docElementName]*regexp.Regexp),
		frontmatterReg: regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
	}

	switch fileExt {
	case mdExt:
		doc.elementRegs[elementH1Block] = regexp.MustCompile(`(?:^|\n)\#{1}\s?([^#][\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.elementRegs[elementH2Block] = regexp.MustCompile(`(?:^|\n)\#{2}\s?([^#][\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.elementStripRegs = DocRegs{
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
			regexp.MustCompile(`\!\[(?:.*?)\]\s?[\[\(].*?[\]\)]`),
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
		doc.elementRegs[elementH1Block] = regexp.MustCompile(`(?m)(?:^|\n)={1}\s?([^=][\S\s]+?)\n=`)
		doc.elementRegs[elementH2Block] = regexp.MustCompile(`(?m)(?:^|\n)={2}\s?([^=][\S\s]+?)\n=`)
		doc.elementStripRegs = DocRegs{
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
	rawContent := string(contentByte)

	return newDoc(rawContent, fileExt), nil
}

func (doc *Doc) Title() string {
	if title := doc.frontmatter(docTitle); title != "" {
		return title
	}

	return doc.element(docTitle)
}

func (doc *Doc) Description(maxLenght int) string {
	desc := doc.frontmatter(docDescription)

	if desc == "" {
		desc = doc.element(docDescription)
	}

	if maxLenght == 0 {
		return desc
	}

	var (
		sentences = strings.Split(desc, ".")
		symbols   int
	)

	for i, sentence := range sentences {
		symbols += len(sentence)

		if symbols > maxLenght {
			if i == 0 {
				desc = sentence
			} else {
				desc = strings.Join(sentences[:i], ".")
			}

			desc += "."
			break
		}
	}

	return desc
}

func (doc *Doc) Content(raw bool) string {
	if raw {
		return doc.rawContent
	}

	// strip doc elements
	str := doc.elementStripRegs.Replace(doc.rawContent)

	// remove redundant spaces and new lines
	str = strings.Join(strings.Fields(str), " ")

	return str
}

func (doc *Doc) frontmatter(key docDataKey) string {
	if doc.frontmatterReg == nil {
		return ""
	}

	if doc.frontmatterCache == nil {
		doc.frontmatterCache = make(map[docDataKey]string)

		match := doc.frontmatterReg.FindStringSubmatch(doc.rawContent)
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

func (doc *Doc) element(key docDataKey) string {
	if doc.elementRegs == nil {
		return ""
	}

	if doc.elementCache == nil {
		doc.elementCache = make(map[docDataKey]string)

		for elementName, elementReg := range doc.elementRegs {
			match := elementReg.FindStringSubmatch(doc.rawContent)
			if len(match) == 0 {
				continue
			}
			lines := strings.Split(match[1], "\n")

			switch elementName {
			case elementH1Block:
				// header title
				str := lines[0]

				doc.elementCache[docTitle] = str
				fallthrough

			case elementH2Block:
				if len(lines) > 1 {
					// header body
					str := strings.Join(lines[1:], "\n")

					// strip doc elements
					str = doc.elementStripRegs.Replace(str)

					str = doc.elementCache[docDescription] + " " + str

					// remove redundant spaces and new lines
					str = strings.Join(strings.Fields(str), " ")

					doc.elementCache[docDescription] = str
				}
			}
		}
	}

	return doc.elementCache[key]
}
