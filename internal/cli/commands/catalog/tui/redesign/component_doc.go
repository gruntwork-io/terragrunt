package redesign

import (
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
	tagCache           map[docDataKey]string
	tagRegs            map[docTagName]*regexp.Regexp
	frontmatterCache   map[docDataKey]string
	frontmatterTags    []string
	frontmatterReg     *regexp.Regexp
	frontmatterDashReg *regexp.Regexp
	frontmatterBody    string
	rawContent         string
	fileExt            string
	tagStripRegs       docRegs
	frontmatterDone    bool
}

// NewComponentDoc builds a ComponentDoc from raw README content.
func NewComponentDoc(rawContent, fileExt string) *ComponentDoc {
	doc := &ComponentDoc{
		rawContent: rawContent,
		fileExt:    fileExt,

		tagRegs:            make(map[docTagName]*regexp.Regexp),
		frontmatterReg:     regexp.MustCompile(`(?i)^[\s\n]*<!-- frontmatter[\s\n]*([\S\s]*?)[\s\n]*-->`),
		frontmatterDashReg: regexp.MustCompile(`(?m)\A[\s\n]*---[\s\n]+([\S\s]*?)[\s\n]+---(?:[\s\n]|$)`),
	}

	doc.extractFrontmatter()

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

// FindComponentDoc reads the first README-like file in dir from fsys and
// returns a populated ComponentDoc. Returns a zero-value *ComponentDoc
// (non-nil) when no README is present.
func FindComponentDoc(fsys vfs.FS, dir string) (*ComponentDoc, error) {
	files, err := vfs.ReadDirEntries(fsys, dir)
	if err != nil {
		return nil, errors.New(err)
	}

	var filePath, fileExt string

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

	contentByte, err := vfs.ReadFile(fsys, filePath)
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

// Tags returns the list of tags declared in the README front-matter, in
// authoring order. Returns nil when no tags are defined or no front-matter
// is present.
func (doc *ComponentDoc) Tags() []string {
	doc.ensureFrontmatter()

	if len(doc.frontmatterTags) == 0 {
		return nil
	}

	out := make([]string, len(doc.frontmatterTags))
	copy(out, doc.frontmatterTags)

	return out
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
	doc.ensureFrontmatter()

	return doc.frontmatterCache[key]
}

// ensureFrontmatter parses the README front-matter block as YAML on first
// use, populating frontmatterCache (name/description) and frontmatterTags.
//
// Unknown keys and parse errors are silently ignored.
func (doc *ComponentDoc) ensureFrontmatter() {
	if doc.frontmatterDone {
		return
	}

	doc.frontmatterDone = true
	doc.frontmatterCache = make(map[docDataKey]string)

	if doc.frontmatterBody == "" {
		return
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(doc.frontmatterBody), &raw); err != nil || raw == nil {
		return
	}

	for k, v := range raw {
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "name":
			if s, ok := v.(string); ok {
				doc.frontmatterCache[docTitle] = strings.TrimSpace(s)
			}
		case "description":
			if s, ok := v.(string); ok {
				doc.frontmatterCache[docDescription] = strings.TrimSpace(s)
			}
		case "tags":
			doc.frontmatterTags = coerceTags(v)
		}
	}
}

// extractFrontmatter captures the YAML body of the README's front-matter
// block (if any) and removes the matched block from rawContent so downstream
// rendering (glamour, tag-stripping) does not treat the front-matter as part
// of the README body. Either the dash-separated form (`---\n...\n---`) or
// the HTML-comment-wrapped form (`<!-- Frontmatter ... -->`) is accepted.
func (doc *ComponentDoc) extractFrontmatter() {
	for _, reg := range []*regexp.Regexp{doc.frontmatterDashReg, doc.frontmatterReg} {
		if reg == nil {
			continue
		}

		loc := reg.FindStringSubmatchIndex(doc.rawContent)
		if len(loc) == 0 {
			continue
		}

		doc.frontmatterBody = doc.rawContent[loc[2]:loc[3]]
		doc.rawContent = strings.TrimLeft(doc.rawContent[loc[1]:], "\r\n")

		return
	}
}

// coerceTags accepts the YAML-decoded value of the `tags` key and returns a
// trimmed, non-empty slice of strings. It accepts either a sequence
// (`["a","b"]` or a `- a` block) or a single string.
func coerceTags(v any) []string {
	switch val := v.(type) {
	case []any:
		out := make([]string, 0, len(val))

		for _, item := range val {
			s, ok := item.(string)
			if !ok {
				continue
			}

			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}

			out = append(out, s)
		}

		return out
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}

		return []string{s}
	}

	return nil
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
