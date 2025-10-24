package format_test

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	logformat "github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terratest/modules/files"
)

func BenchmarkFormat(b *testing.B) {
	tmpBase, err := files.CopyFolderToTemp("./testdata/fixtures", b.Name(), func(path string) bool { return true })
	if err != nil {
		b.Fatalf("Failed to copy fixtures: %v", err)
	}
	defer os.RemoveAll(tmpBase)

	if err = duplicateFixtures(tmpBase, 10); err != nil {
		b.Fatalf("Failed to duplicate fixtures: %v", err)
	}

	pristineDir := filepath.Join(tmpBase, "pristine")
	if err = os.MkdirAll(pristineDir, 0755); err != nil {
		b.Fatalf("Failed to create pristine dir: %v", err)
	}

	entries, err := os.ReadDir(tmpBase)
	if err != nil {
		b.Fatalf("Failed to read tmpBase: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() == "pristine" || entry.Name()[0] == '.' {
			continue
		}

		src := filepath.Join(tmpBase, entry.Name())
		dst := filepath.Join(pristineDir, entry.Name())

		if err = filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}

			relPath, relErr := filepath.Rel(src, path)
			if relErr != nil {
				return relErr
			}

			destPath := filepath.Join(dst, relPath)

			if d.IsDir() {
				info, infoErr := d.Info()
				if infoErr != nil {
					return infoErr
				}
				return os.MkdirAll(destPath, info.Mode())
			}

			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}

			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.WriteFile(destPath, content, info.Mode())
		}); err != nil {
			b.Fatalf("Failed to copy to pristine: %v", err)
		}
	}

	workingDir := tmpBase

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	if err != nil {
		b.Fatalf("Failed to create options: %v", err)
	}

	tgOptions.WorkingDir = workingDir
	tgOptions.HclExclude = []string{".history", "pristine"}
	tgOptions.FilterQueries = []string{}
	tgOptions.Experiments = experiment.NewExperiments()
	tgOptions.Writer = io.Discard
	tgOptions.ErrWriter = io.Discard

	formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)
	l := log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel), log.WithFormatter(formatter))
	ctx := context.Background()

	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()
		if err := resetFixturesToUnformatted(pristineDir, workingDir); err != nil {
			b.Fatalf("Failed to reset fixtures: %v", err)
		}
		b.StartTimer()

		if err := format.Run(ctx, l, tgOptions); err != nil {
			b.Fatalf("format.Run failed: %v", err)
		}
	}
}

// duplicateFixtures creates multiple copies of the fixtures directory structure to increase
// the number of files for more realistic benchmarking.
func duplicateFixtures(baseDir string, count int) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return err
	}

	var origDirs []string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name()[0] != '.' {
			origDirs = append(origDirs, filepath.Join(baseDir, entry.Name()))
		}
	}

	for i := 1; i < count; i++ {
		for _, origDir := range origDirs {
			newDirName := filepath.Base(origDir) + "-dup-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
			newDir := filepath.Join(baseDir, newDirName)

			err := filepath.WalkDir(origDir, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}

				relPath, relErr := filepath.Rel(origDir, path)
				if relErr != nil {
					return relErr
				}

				newPath := filepath.Join(newDir, relPath)

				if d.IsDir() {
					info, infoErr := d.Info()
					if infoErr != nil {
						return infoErr
					}
					return os.MkdirAll(newPath, info.Mode())
				}

				content, readErr := os.ReadFile(path)
				if readErr != nil {
					return readErr
				}

				info, infoErr := d.Info()
				if infoErr != nil {
					return infoErr
				}
				return os.WriteFile(newPath, content, info.Mode())
			})

			if err != nil {
				return err
			}
		}
	}

	return nil
}

// resetFixturesToUnformatted copies the pristine unformatted fixtures back to the working directory
func resetFixturesToUnformatted(pristineDir, workingDir string) error {
	pristineDirName := filepath.Base(pristineDir)

	entries, err := os.ReadDir(workingDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Name()[0] == '.' || entry.Name() == pristineDirName {
			continue
		}

		path := filepath.Join(workingDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	return filepath.WalkDir(pristineDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, relErr := filepath.Rel(pristineDir, path)
		if relErr != nil {
			return relErr
		}

		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(workingDir, relPath)

		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(destPath, info.Mode())
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		return os.WriteFile(destPath, content, info.Mode())
	})
}
