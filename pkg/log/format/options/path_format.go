package options

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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

var pathFormatList = NewMapValue(map[PathFormatValue]string{ //nolint:gochecknoglobals
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
// For better performance, during instance creation, we creating a cache of relative paths for each subdirectory of baseDir.
//
// Example of cache:
// /path/to/dir ./
// /path/to     ../
// /path        ../..
type RelativePather struct {
	relPaths    []string
	absPathsReg []*regexp.Regexp
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
	absPathsReg := make([]*regexp.Regexp, len(dirs))
	reversIndex := len(dirs)

	for _, dir := range dirs {
		absPath = filepath.Join(absPath, pathSeparator, dir)

		relPath, err := filepath.Rel(baseDir, absPath)
		if err != nil {
			return nil, errors.New(err)
		}

		reversIndex--
		relPaths[reversIndex] = relPath

		regStr := fmt.Sprintf(`(^|[^%[1]s\w])%[2]s([%[1]s"'\s]|$)`, regexp.QuoteMeta(pathSeparator), regexp.QuoteMeta(absPath))
		absPathsReg[reversIndex] = regexp.MustCompile(regStr)
	}

	return &RelativePather{
		absPathsReg: absPathsReg,
		relPaths:    relPaths,
	}, nil
}

func (hook *RelativePather) ReplaceAbsPaths(str string) string {
	for i, absPath := range hook.absPathsReg {
		str = absPath.ReplaceAllString(str, "$1"+hook.relPaths[i]+"$2")
	}

	return str
}
