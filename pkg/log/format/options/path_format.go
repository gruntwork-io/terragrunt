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

func replaceStandalonePath(str, absPath, relPath string) string {
	var (
		b        strings.Builder
		last     int
		replaced bool
	)

	for i := 0; i < len(str); {
		j := strings.Index(str[i:], absPath)
		if j < 0 {
			break
		}

		start := i + j
		end := start + len(absPath)

		if !isPathStart(str, start) || !isPathEnd(str, end) {
			i = start + 1
			continue
		}

		b.WriteString(str[last:start])
		b.WriteString(relPath)

		last, i, replaced = end, end, true
	}

	if !replaced {
		return str
	}

	b.WriteString(str[last:])

	return b.String()
}

func isPathStart(str string, start int) bool {
	if start == 0 {
		return true
	}

	c := str[start-1]

	return c != filepath.Separator && !isWordByte(c)
}

func isPathEnd(str string, end int) bool {
	if end == len(str) {
		return true
	}

	switch str[end] {
	case filepath.Separator, '"', '\'', ' ', '\t', '\n', '\f', '\r':
		return true
	}

	return false
}

func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}
