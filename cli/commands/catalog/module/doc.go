package module

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	mdExt   = ".md"
	adocExt = ".adoc"

	docTitle docDataKey = iota
	docDescription
	docContent

	tagH1Block docTagName = iota
	tagH2Block
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
type docTagName byte

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

	tagCache     map[docDataKey]string
	tagRegs      map[docTagName]*regexp.Regexp
	tagStripRegs DocRegs

	frontmatterCache map[docDataKey]string
	frontmatterReg   *regexp.Regexp
}

func NewDoc(rawContent, fileExt string) *Doc {
	doc := &Doc{
		rawContent: rawContent,
		fileExt:    fileExt,

		tagRegs:        make(map[docTagName]*regexp.Regexp),
		frontmatterReg: regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
	}

	switch fileExt {
	case mdExt:
		doc.tagRegs[tagH1Block] = regexp.MustCompile(`(?:^|\n)\#{1}\s([\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.tagRegs[tagH2Block] = regexp.MustCompile(`(?:^|\n)\#{2}\s([\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.tagStripRegs = DocRegs{
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
		doc.tagRegs[tagH1Block] = regexp.MustCompile(`(?:^|\n)\={1}\s([\S\s]+?)(?:[\r\n]+\=|[\r\n]*$)`)
		doc.tagRegs[tagH2Block] = regexp.MustCompile(`(?:^|\n)\={2}\s([\S\s]+?)(?:[\r\n]+\=|[\r\n]*$)`)
		doc.tagStripRegs = DocRegs{
			// html
			regexp.MustCompile("<(.*?)>"),
			// ifdef endif
			regexp.MustCompile(`ifdef::env-github\[\][\S\s]*?endif::\[\]`),
			// comment
			regexp.MustCompile(`(?m)^/{2}.*$`),
			// ex. :name:value
			regexp.MustCompile(`(?m)^:[-!\w]+:.*$`),
			// ex. toc::[]
			regexp.MustCompile(`\w+::\[.*?\]`),
			// bold
			regexp.MustCompile(`\*\*([^\s][^*]+[^\s])\*\*`),
			regexp.MustCompile(`\*([^\s][^*]+[^\s])\*`),
			// italic
			regexp.MustCompile(`_{1,2}([^\s][^_]+[^\s])_{1,2}`),
			// image
			regexp.MustCompile(`image:[^\]]+]`),
			// link
			regexp.MustCompile(`(?:link|https):[\S\s]+?\[([\S\s]+?)\]`),
			// header
			regexp.MustCompile(`(?m)^\={1,6}\s*([^=]+)\s*(\={1,6})?$`),
			// multiple line break
			regexp.MustCompile(`((?:\r\n?|\n){2})(?:\r\n?|\n)*`),
		}
	}

	return doc
}

func FindDoc(dir string) (*Doc, error) {
	var filePath, fileExt string

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New(err)
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
		return nil, errors.New(err)
	}

	rawContent := string(contentByte)

	return NewDoc(rawContent, fileExt), nil
}

func (doc *Doc) Title() string {
	if title := doc.parseFrontmatter(docTitle); title != "" {
		return title
	}

	return doc.parseTag(docTitle)
}

func (doc *Doc) Description(maxLenght int) string {
	desc := doc.parseFrontmatter(docDescription)

	if desc == "" {
		desc = doc.parseTag(docDescription)
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

func (doc *Doc) Content(stripTags bool) string {
	if !stripTags {
		return doc.rawContent
	}

	return doc.parseTag(docContent)
}

func (doc *Doc) IsMarkDown() bool {
	return doc.fileExt == mdExt
}

// parseFrontmatter parses Markdown files with frontmatter, which we use as the preferred title/description source.
func (doc *Doc) parseFrontmatter(key docDataKey) string {
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

// parseTag parses Markdown/AsciiDoc files, stips tags and extracts the H1 header as the title and the H1+H2 bodies as the description.
func (doc *Doc) parseTag(key docDataKey) string {
	if doc.tagRegs == nil {
		return ""
	}

	if doc.tagCache == nil {
		doc.tagCache = make(map[docDataKey]string)

		var h1Body, h2Body string

		for tagName, tagReg := range doc.tagRegs {
			match := tagReg.FindStringSubmatch(doc.rawContent)
			if len(match) == 0 {
				continue
			}

			lines := strings.Split(match[1], "\n")

			switch tagName {
			case tagH1Block:
				// header title
				doc.tagCache[docTitle] = lines[0]

				if len(lines) > 1 {
					h1Body = strings.Join(lines[1:], "\n")
				}

			case tagH2Block:
				if len(lines) > 1 {
					h2Body = strings.Join(lines[1:], "\n")
				}
			}
		}

		desc := h1Body + " " + h2Body

		// strip doc tags
		desc = doc.tagStripRegs.Replace(desc)

		// remove redundant spaces and new lines
		desc = strings.Join(strings.Fields(desc), " ")

		doc.tagCache[docDescription] = desc

		// strip doc tags
		content := doc.tagStripRegs.Replace(doc.rawContent)

		doc.tagCache[docContent] = content
	}

	return doc.tagCache[key]
}
