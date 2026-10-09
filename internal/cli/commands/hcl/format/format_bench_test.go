package format_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	logformat "github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

func BenchmarkFormat(b *testing.B) {
	sourceFile := "../../../../../test/fixtures/hcl-filter/fmt/needs-formatting/nested/api/terragrunt.hcl"

	pristineContent, err := os.ReadFile(sourceFile)
	require.NoError(b, err, "Failed to read source file")

	fileCounts := []int{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024}

	for _, fileCount := range fileCounts {
		b.Run(fmt.Sprintf("files_%d", fileCount), func(b *testing.B) {
			tmpBase := b.TempDir()

			var excludeList []string
			for i := 2; i <= fileCount; i += 2 {
				excludeList = append(excludeList, fmt.Sprintf("dir-%04d", i))
			}

			tgOptions, err := options.NewTerragruntOptionsForTest("")
			require.NoError(b, err, "Failed to create options")

			tgOptions.WorkingDir = tmpBase
			tgOptions.HclExclude = excludeList
			v := venvtest.NewWithOSFS()

			formatter := logformat.NewFormatter(logformat.NewKeyValueFormatPlaceholders())
			formatter.SetDisabledColors(true)
			l := log.New(
				log.WithOutput(io.Discard),
				log.WithLevel(log.ErrorLevel),
				log.WithFormatter(formatter),
			)
			ctx := context.Background()

			for b.Loop() {
				b.StopTimer()

				require.NoError(b, createFiles(tmpBase, pristineContent, fileCount), "Failed to create files")

				b.StartTimer()

				require.NoError(b, format.Run(ctx, l, v, tgOptions), "format.Run failed")
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
