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

const PathFormatOptionName = "path"

const (
	NonePath PathFormatValue = iota
	RelativePath
	RelativeModulePath
	ModulePath
	FilenamePath
	DirectoryPath
)

var pathFormatValues = CommonMapValues[PathFormatValue]{
	RelativePath:       "relative",
	RelativeModulePath: "relative-module",
	ModulePath:         "module",
	FilenamePath:       "filename",
	DirectoryPath:      "dir",
}

type PathFormatValue byte

type pathFormat struct {
	*CommonOption[PathFormatValue]
}

func (option *pathFormat) Evaluate(data *Data, str string) string {
	switch option.value {
	case RelativePath:
		if data.RelativePather == nil {
			break
		}

		return data.RelativePather.ReplaceAbsPaths(str)
	case RelativeModulePath:
		if data.RelativePather == nil {
			break
		}

		str = data.RelativePather.ReplaceAbsPaths(str)

		if str == log.CurDir {
			return ""
		}

		if strings.HasPrefix(str, log.CurDirWithSeparator) {
			return str[len(log.CurDirWithSeparator):]
		}

		return str
	case ModulePath:
		if str == data.BaseDir {
			return ""
		}

		return str
	case FilenamePath:
		return filepath.Base(str)
	case DirectoryPath:
		return filepath.Dir(str)
	case NonePath:
	}

	return str
}

func PathFormat(val PathFormatValue, allowed ...PathFormatValue) Option {
	values := pathFormatValues
	if len(allowed) > 0 {
		values = values.Filter(allowed...)
	}

	return &pathFormat{
		CommonOption: NewCommonOption[PathFormatValue](PathFormatOptionName, val, values),
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
