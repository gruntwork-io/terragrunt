package options

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// PathFormatOptionName is the option name.
const PathFormatOptionName = "path"

const (
	NonePath PathFormatValue = iota
	RelativePath
	ShortRelativePath
	ShortPath
	FilenamePath
	DirectoryPath
)

var pathFormatList = NewMapValue(map[PathFormatValue]string{
	RelativePath:      "relative",
	ShortRelativePath: "short-relative",
	ShortPath:         "short",
	FilenamePath:      "filename",
	DirectoryPath:     "dir",
})

type PathFormatValue byte

type PathFormatOption struct {
	*CommonOption[PathFormatValue]
}

// Format implements `Option` interface.
func (option *PathFormatOption) Format(data *Data, val any) (any, error) {
	str := toString(val)

	switch option.value.Get() {
	case RelativePath:
		if data.RelativePather == nil {
			break
		}

		return data.RelativePather.ReplaceAbsPaths(str), nil
	case ShortRelativePath:
		if data.RelativePather == nil {
			break
		}

		return option.shortRelativePath(data, str), nil
	case ShortPath:
		if str == data.BaseDir {
			return "", nil
		}

		return str, nil
	case FilenamePath:
		return filepath.Base(str), nil
	case DirectoryPath:
		return filepath.Dir(str), nil
	case NonePath:
	}

	return val, nil
}

func (option *PathFormatOption) shortRelativePath(data *Data, str string) string {
	if str == data.BaseDir {
		return ""
	}

	str = data.RelativePather.ReplaceAbsPaths(str)

	if strings.HasPrefix(str, log.CurDirWithSeparator) {
		return str[len(log.CurDirWithSeparator):]
	}

	return str
}

// PathFormat creates the option to format the paths.
func PathFormat(val PathFormatValue, allowed ...PathFormatValue) Option {
	list := pathFormatList
	if len(allowed) > 0 {
		list = list.Filter(allowed...)
	}

	return &PathFormatOption{
		CommonOption: NewCommonOption(PathFormatOptionName, list.Set(val)),
	}
}

// RelativePather replaces absolute paths with relative ones,
// For better performance, during instance creation, we creating a cache
// of relative paths for each subdirectory of baseDir.
//
// Example of cache:
// /path/to/dir ./
// /path/to     ../
// /path        ../..
type RelativePather struct {
	absPaths []string
	relPaths []string
}

// NewRelativePather returns a new RelativePather instance.
// It returns an error if the cache of relative paths could not be created for the given `baseDir`.
func NewRelativePather(baseDir string) (*RelativePather, error) {
	baseDir = filepath.Clean(baseDir)

	pathSeparator := string(os.PathSeparator)
	dirs := strings.Split(baseDir, pathSeparator)
	absPath := dirs[0]
	dirs = dirs[1:]

	relPaths := make([]string, len(dirs))
	absPaths := make([]string, len(dirs))
	reversIndex := len(dirs)

	for _, dir := range dirs {
		absPath = filepath.Join(absPath, pathSeparator, dir)

		relPath, err := filepath.Rel(baseDir, absPath)
		if err != nil {
			return nil, err
		}

		reversIndex--
		relPaths[reversIndex] = relPath
		absPaths[reversIndex] = absPath
	}

	return &RelativePather{
		absPaths: absPaths,
		relPaths: relPaths,
	}, nil
}

// ReplaceAbsPaths rewrites every cached absolute path in str to its relative
// form, deepest directory first. A path is only replaced where it stands alone:
// the byte before it is not a path separator or a word character, and the byte
// after it ends the string or is a path separator, a quote, or whitespace.
func (hook *RelativePather) ReplaceAbsPaths(str string) string {
	for i, absPath := range hook.absPaths {
		str = replaceStandalonePath(str, absPath, hook.relPaths[i])
	}

	return str
}

// replaceStandalonePath returns str with every standalone occurrence of
// absPath replaced by relPath.
func replaceStandalonePath(str, absPath, relPath string) string {
	at := indexStandalone(str, absPath, 0)
	if at < 0 {
		return str
	}

	var replaced strings.Builder

	copied := 0

	for at >= 0 {
		replaced.WriteString(str[copied:at])
		replaced.WriteString(relPath)

		copied = at + len(absPath)
		at = indexStandalone(str, absPath, copied)
	}

	replaced.WriteString(str[copied:])

	return replaced.String()
}

// indexStandalone returns the index of the first standalone occurrence of
// absPath in str at or after from, and -1 when there is none.
func indexStandalone(str, absPath string, from int) int {
	for at := from; at < len(str); {
		i := strings.Index(str[at:], absPath)
		if i < 0 {
			return -1
		}

		at += i

		if standsAlone(str, at, at+len(absPath)) {
			return at
		}

		at++
	}

	return -1
}

// standsAlone reports whether str[start:end] holds a path of its own rather
// than a leading piece of a longer path or a word. A separator or a word
// character before it makes it part of something longer, and so does anything
// but a separator, a quote, or whitespace after it.
func standsAlone(str string, start, end int) bool {
	if start > 0 && (str[start-1] == filepath.Separator || isWordByte(str[start-1])) {
		return false
	}

	return end == len(str) || isPathDelimiter(str[end])
}

// isPathDelimiter reports whether c can follow a path in a log line. A
// separator counts: the path is then the parent of a longer one, which is
// replaced along with it.
func isPathDelimiter(c byte) bool {
	switch c {
	case filepath.Separator, '"', '\'', ' ', '\t', '\n', '\f', '\r':
		return true
	}

	return false
}

func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
