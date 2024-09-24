// Package hooks provides hooks for the Terragrunt logger.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/sirupsen/logrus"
)

// RelativePathHook represents a hook for logrus logger.
// The purpose is to replace all absolute paths found in the main message or data fields with paths relative to `baseDir`.
// For better performance, during instance creation, we creating a cache of relative paths for each subdirectory of baseDir.
//
// Example of cache:
// /path/to/dir ./
// /path/to     ../
// /path        ../..
type RelativePathHook struct {
	relPaths      []string
	absPathsReg   []*regexp.Regexp
	triggerLevels []logrus.Level
}

// NewRelativePathHook returns a new RelativePathHook instance.
// It returns an error if the cache of relative paths could not be created for the given `baseDir`.
func NewRelativePathHook(baseDir string) (*RelativePathHook, error) {
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
			return nil, errors.WithStackTrace(err)
		}

		reversIndex--
		relPaths[reversIndex] = relPath
		absPathsReg[reversIndex] = regexp.MustCompile(fmt.Sprintf(`(^|[^%[1]s\w])%[2]s([%[1]s"'\s]|$)`, regexp.QuoteMeta(pathSeparator), regexp.QuoteMeta(absPath)))
	}

	return &RelativePathHook{
		absPathsReg:   absPathsReg,
		relPaths:      relPaths,
		triggerLevels: log.AllLevels.ToLogrusLevels(),
	}, nil
}

// Levels implements logrus.Hook.Levels()
func (hook *RelativePathHook) Levels() []logrus.Level {
	return hook.triggerLevels
}

// Fire implements logrus.Hook.Fire()
func (hook *RelativePathHook) Fire(entry *logrus.Entry) error {
	entry.Message = hook.replaceAbsPathsWithRel(entry.Message)

	for key, field := range entry.Data {
		if val, ok := field.(string); ok {
			newVal := hook.replaceAbsPathsWithRel(val)

			if newVal == val {
				continue
			}

			if key == format.PrefixKeyName && strings.HasPrefix(newVal, log.CurDirWithSeparator) {
				newVal = newVal[len(log.CurDirWithSeparator):]
			}

			entry.Data[key] = newVal
		}
	}

	return nil
}

func (hook *RelativePathHook) replaceAbsPathsWithRel(text string) string {
	for i, absPath := range hook.absPathsReg {
		text = absPath.ReplaceAllString(text, "$1"+hook.relPaths[i]+"$2")
	}

	return text
}
