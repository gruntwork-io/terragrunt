package hooks

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// RelativePathHook represents a hook for logrus logger.
// The purpose is to replace all absolute paths found in the main message or data fields with paths relative to `baseDir`.
// For better performance, during instance creation, we creating a cache of relative paths for each subdirectory of baseDir.
//
// Example of cache:
// /path/to/dir ./
// /path/to     ../
// /path        ../..
//
// This way, using the standard `strings.ReplaceAll`, we can replace absolute paths with relative ones for different lengths, iterating from longest to shortest.
type RelativePathHook struct {
	// absPaths are the keys of the `realPath` map, we store them in the order we iterate over them when replacing.
	absPaths      []string
	relPaths      map[string]string
	triggerLevels []logrus.Level
}

// NewRelativePathHook returns a new RelativePathHook instance.
// It returns an error if the cache of relative paths could not be created for the given `baseDir`.
func NewRelativePathHook(baseDir string) (*RelativePathHook, error) {
	baseDir = filepath.Clean(baseDir)
	relPaths := make(map[string]string)

	dirs := strings.Split(baseDir, string(os.PathSeparator))
	absPath := dirs[0]
	for _, dir := range dirs[1:] {
		absPath = filepath.Join(absPath, string(os.PathSeparator), dir)

		relPath, err := filepath.Rel(baseDir, absPath)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}

		// if relPath is the current directory `.`, we add PathSeperator `./` to avoid confusion with the dot at the end of the sentence.
		if relPath == log.CurDir {
			relPath = log.CurDirWithSeparator
			relPaths[absPath+string(os.PathSeparator)] = log.CurDirWithSeparator
		}

		relPaths[absPath] = relPath
	}

	absPaths := maps.Keys(relPaths)
	slices.SortFunc(absPaths, func(a, b string) int {
		if a > b {
			return -1
		}
		return 0
	})

	return &RelativePathHook{
		absPaths:      absPaths,
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

	// for key, field := range entry.Data {
	// 	if val, ok := field.(string); ok {
	// 		var (
	// 			val    = val
	// 			newVal = hook.replaceAbsPathsWithRel(val)
	// 		)

	// 		if newVal == val {
	// 			continue
	// 		}

	// 		if key == log.FieldKeyPrefix {
	// 			entry.Data[key] = func() (string, string) {
	// 				return val, newVal
	// 			}
	// 			continue
	// 		}

	// 		entry.Data[key] = newVal
	// 	}
	// }

	return nil
}

func (hook *RelativePathHook) replaceAbsPathsWithRel(text string) string {
	for _, absPath := range hook.absPaths {
		text = strings.ReplaceAll(text, absPath, hook.relPaths[absPath])
	}
	return text
}
