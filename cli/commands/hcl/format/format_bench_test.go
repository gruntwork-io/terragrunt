package format_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	logformat "github.com/gruntwork-io/terragrunt/pkg/log/format"
)

func BenchmarkFormat(b *testing.B) {
	sourceFile := "../../../../test/fixtures/hcl-filter/fmt/needs-formatting/nested/api/terragrunt.hcl"

	pristineContent, err := os.ReadFile(sourceFile)
	if err != nil {
		b.Fatalf("Failed to read source file: %v", err)
	}

	fileCounts := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("files_%d", fileCount), func(b *testing.B) {
			tmpBase := b.TempDir()

			var excludeList []string
			for i := 2; i <= fileCount; i += 2 {
				excludeList = append(excludeList, fmt.Sprintf("dir-%04d", i))
			}

			tgOptions, err := options.NewTerragruntOptionsForTest("")
			if err != nil {
				b.Fatalf("Failed to create options: %v", err)
			}

			tgOptions.WorkingDir = tmpBase
			tgOptions.HclExclude = excludeList
			tgOptions.Writer = io.Discard
			tgOptions.ErrWriter = io.Discard

			formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
			formatter.SetDisabledColors(true)
			l := log.New(log.WithOutput(io.Discard), log.WithLevel(log.ErrorLevel), log.WithFormatter(formatter))
			ctx := context.Background()

			b.ResetTimer()

			for b.Loop() {
				b.StopTimer()

				if err := createFiles(tmpBase, pristineContent, fileCount); err != nil {
					b.Fatalf("Failed to create files: %v", err)
				}

				b.StartTimer()

				if err := format.Run(ctx, l, tgOptions); err != nil {
					b.Fatalf("format.Run failed: %v", err)
				}
			}
		})
	}
}

func createFiles(workingDir string, content []byte, count int) error {
	entries, err := os.ReadDir(workingDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "dir-") {
			if err := os.RemoveAll(filepath.Join(workingDir, entry.Name())); err != nil {
				return err
			}
		}
	}

	for i := 1; i <= count; i++ {
		dirName := fmt.Sprintf("dir-%04d", i)
		dirPath := filepath.Join(workingDir, dirName)

		nestedPath := filepath.Join(dirPath, "nested", "deep", "structure")
		if err := os.MkdirAll(nestedPath, 0755); err != nil {
			return err
		}

		filePath := filepath.Join(nestedPath, "terragrunt.hcl")
		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return err
		}
	}

	return nil
}
