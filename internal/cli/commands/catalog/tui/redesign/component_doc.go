package redesign

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Note: this file is a redesign-owned fork of internal/services/catalog/module/doc.go.
// It is intentionally duplicated to keep the redesign path isolated from the
// legacy catalog pipeline.

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
	docFiles = []string{"README.md", "README.adoc"}

	frontmatterKeys = map[string]docDataKey{
		"name":        docTitle,
		"description": docDescription,
	}
)

type docDataKey byte
type docTagName byte

type docRegs []*regexp.Regexp

func (regs docRegs) Replace(str string) string {
	for _, reg := range regs {
		str = reg.ReplaceAllString(str, "$1")
	}

	return str
}

// ComponentDoc is the parsed README (Markdown or AsciiDoc) for a Component.
type ComponentDoc struct {
	tagCache         map[docDataKey]string
	tagRegs          map[docTagName]*regexp.Regexp
	frontmatterCache map[docDataKey]string
	frontmatterReg   *regexp.Regexp
	rawContent       string
	fileExt          string
	tagStripRegs     docRegs
}

// NewComponentDoc builds a ComponentDoc from raw README content.
func NewComponentDoc(rawContent, fileExt string) *ComponentDoc {
	doc := &ComponentDoc{
		rawContent: rawContent,
		fileExt:    fileExt,

		tagRegs:        make(map[docTagName]*regexp.Regexp),
		frontmatterReg: regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
	}

	switch fileExt {
	case mdExt:
		doc.tagRegs[tagH1Block] = regexp.MustCompile(`(?:^|\n)\#{1}\s([\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.tagRegs[tagH2Block] = regexp.MustCompile(`(?:^|\n)\#{2}\s([\S\s]+?)(?:[\r\n]+\#|[\r\n]*$)`)
		doc.tagStripRegs = docRegs{
			regexp.MustCompile("`{3}" + `.*[\r\n]+`),
			regexp.MustCompile("`(.+?)`"),
			regexp.MustCompile("<(.*?)>"),
			regexp.MustCompile(`\*\*([^*]+)\*\*`),
			regexp.MustCompile(`__([^_]+)__`),
			regexp.MustCompile(`\*([^*]+)\*`),
			regexp.MustCompile(`_([^_]+)_`),
			regexp.MustCompile(`^[=\-]{2,}\s*$`),
			regexp.MustCompile(`\[\^.+?\](\: .*?$)?`),
			regexp.MustCompile(`\s{0,2}\[.*?\]: .*?$`),
			regexp.MustCompile(`\!\[(?:.*?)\]\s?[\[\(].*?[\]\)]`),
			regexp.MustCompile(`\[([\S\s]*?)\][\[\(].*?[\]\)]`),
			regexp.MustCompile(`>\s*`),
			regexp.MustCompile(`^\s{1,2}\[(.*?)\]: (\S+)( ".*?")?\s*$`),
			regexp.MustCompile(`(?m)^\#{1,6}\s*([^#]+)\s*(\#{1,6})?$`),
			regexp.MustCompile(`^[-\*_]{3,}\s*$`),
		}

	case adocExt:
		doc.tagRegs[tagH1Block] = regexp.MustCompile(`(?:^|\n)\={1}\s([\S\s]+?)(?:[\r\n]+\=|[\r\n]*$)`)
		doc.tagRegs[tagH2Block] = regexp.MustCompile(`(?:^|\n)\={2}\s([\S\s]+?)(?:[\r\n]+\=|[\r\n]*$)`)
		doc.tagStripRegs = docRegs{
			regexp.MustCompile("<(.*?)>"),
			regexp.MustCompile(`ifdef::env-github\[\][\S\s]*?endif::\[\]`),
			regexp.MustCompile(`(?m)^/{2}.*$`),
			regexp.MustCompile(`(?m)^:[-!\w]+:.*$`),
			regexp.MustCompile(`\w+::\[.*?\]`),
			regexp.MustCompile(`\*\*([^\s][^*]+[^\s])\*\*`),
			regexp.MustCompile(`\*([^\s][^*]+[^\s])\*`),
			regexp.MustCompile(`_{1,2}([^\s][^_]+[^\s])_{1,2}`),
			regexp.MustCompile(`image:[^\]]+]`),
			regexp.MustCompile(`(?:link|https):[\S\s]+?\[([\S\s]+?)\]`),
			regexp.MustCompile(`(?m)^\={1,6}\s*([^=]+)\s*(\={1,6})?$`),
			regexp.MustCompile(`((?:\r\n?|\n){2})(?:\r\n?|\n)*`),
		}
	}

	return doc
}

// FindComponentDoc reads the first README-like file in dir and returns a
// populated ComponentDoc. Returns a zero-value *ComponentDoc (non-nil) when
// no README is present.
func FindComponentDoc(dir string) (*ComponentDoc, error) {
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

		if strings.EqualFold(fileExt, mdExt) {
			break
		}
	}

	if filePath == "" {
		return &ComponentDoc{}, nil
	}

	contentByte, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.New(err)
	}

	return NewComponentDoc(string(contentByte), fileExt), nil
}

// Title returns the doc title from frontmatter or the first H1.
func (doc *ComponentDoc) Title() string {
	if title := doc.parseFrontmatter(docTitle); title != "" {
		return title
	}

	return doc.parseTag(docTitle)
}

// Description returns a short description, optionally capped at maxLength.
func (doc *ComponentDoc) Description(maxLength int) string {
	desc := doc.parseFrontmatter(docDescription)

	if desc == "" {
		desc = doc.parseTag(docDescription)
	}

	if maxLength == 0 {
		return desc
	}

	var (
		sentences = strings.Split(desc, ".")
		symbols   int
	)

	for i, sentence := range sentences {
		symbols += len(sentence)

		if symbols > maxLength {
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

// Content returns the doc content, optionally with tags stripped.
func (doc *ComponentDoc) Content(stripTags bool) string {
	if !stripTags {
		return doc.rawContent
	}

	return doc.parseTag(docContent)
}

// IsMarkDown reports whether the doc is a Markdown README.
func (doc *ComponentDoc) IsMarkDown() bool {
	return doc.fileExt == mdExt
}

func (doc *ComponentDoc) parseFrontmatter(key docDataKey) string {
	if doc.frontmatterReg == nil {
		return ""
	}

	if doc.frontmatterCache == nil {
		doc.frontmatterCache = make(map[docDataKey]string)

		match := doc.frontmatterReg.FindStringSubmatch(doc.rawContent)
		if len(match) == 0 {
			return ""
		}

		lines := strings.SplitSeq(match[1], "\n")

		for line := range lines {
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

func (doc *ComponentDoc) parseTag(key docDataKey) string {
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
		desc = doc.tagStripRegs.Replace(desc)
		desc = strings.Join(strings.Fields(desc), " ")

		doc.tagCache[docDescription] = desc

		content := doc.tagStripRegs.Replace(doc.rawContent)
		doc.tagCache[docContent] = content
	}

	return doc.tagCache[key]
}
