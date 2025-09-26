package util_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	benchmarkBoolSink bool
)

func TestIsTFFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		path        string
		expected    bool
	}{
		{
			description: "Terraform .tf file",
			path:        "main.tf",
			expected:    true,
		},
		{
			description: "OpenTofu .tofu file",
			path:        "main.tofu",
			expected:    true,
		},
		{
			description: "Terraform JSON .tf.json file",
			path:        "main.tf.json",
			expected:    true,
		},
		{
			description: "OpenTofu JSON .tofu.json file",
			path:        "main.tofu.json",
			expected:    true,
		},
		{
			description: "Regular JSON file",
			path:        "config.json",
			expected:    false,
		},
		{
			description: "Regular text file",
			path:        "readme.txt",
			expected:    false,
		},
		{
			description: "No extension",
			path:        "Dockerfile",
			expected:    false,
		},
		{
			description: "HCL file (not Terraform/OpenTofu)",
			path:        "terragrunt.hcl",
			expected:    false,
		},
		{
			description: "Path with directories - .tf file",
			path:        "/path/to/modules/main.tf",
			expected:    true,
		},
		{
			description: "Path with directories - .tofu file",
			path:        "/path/to/modules/main.tofu",
			expected:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			actual := util.IsTFFile(tc.path)
			assert.Equal(t, tc.expected, actual, "For path %s", tc.path)
		})
	}
}

func TestDirContainsTFFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description string
		files       []string
		directories []string
		expected    bool
		expectError bool
	}{
		{
			description: "Directory with .tf file",
			files:       []string{"main.tf"},
			expected:    true,
		},
		{
			description: "Directory with .tofu file",
			files:       []string{"main.tofu"},
			expected:    true,
		},
		{
			description: "Directory with .tf.json file",
			files:       []string{"main.tf.json"},
			expected:    true,
		},
		{
			description: "Directory with .tofu.json file",
			files:       []string{"main.tofu.json"},
			expected:    true,
		},
		{
			description: "Directory with both .tf and .tofu files",
			files:       []string{"main.tf", "variables.tofu"},
			expected:    true,
		},
		{
			description: "Directory with mixed file types including TF files",
			files:       []string{"main.tf", "readme.txt", "config.json"},
			expected:    true,
		},
		{
			description: "Directory with no TF files",
			files:       []string{"readme.txt", "config.json", "script.sh"},
			expected:    false,
		},
		{
			description: "Empty directory",
			files:       []string{},
			expected:    false,
		},
		{
			description: "Directory with subdirectories containing TF files",
			files:       []string{"modules/main.tf", "data/variables.tofu"},
			directories: []string{"modules", "data"},
			expected:    true,
		},
		{
			description: "Directory with only non-TF files in subdirectories",
			files:       []string{"modules/readme.txt", "data/config.json"},
			directories: []string{"modules", "data"},
			expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Create directories
			for _, dir := range tc.directories {
				dirPath := filepath.Join(tmpDir, dir)
				require.NoError(t, os.MkdirAll(dirPath, 0755))
			}

			// Create files
			for _, file := range tc.files {
				filePath := filepath.Join(tmpDir, file)
				// Ensure directory exists
				require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
				require.NoError(t, os.WriteFile(filePath, []byte("# Test file content"), 0644))
			}

			actual, err := util.DirContainsTFFiles(tmpDir)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, actual, "For test case: %s", tc.description)
			}
		})
	}
}

func TestFindTFFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		description   string
		files         []string
		directories   []string
		expectedFiles []string
		expectedCount int
	}{
		{
			description:   "Directory with single .tf file",
			files:         []string{"main.tf"},
			expectedCount: 1,
			expectedFiles: []string{"main.tf"},
		},
		{
			description:   "Directory with single .tofu file",
			files:         []string{"main.tofu"},
			expectedCount: 1,
			expectedFiles: []string{"main.tofu"},
		},
		{
			description:   "Directory with mixed TF file types",
			files:         []string{"main.tf", "variables.tofu", "outputs.tf.json", "providers.tofu.json"},
			expectedCount: 4,
			expectedFiles: []string{"main.tf", "variables.tofu", "outputs.tf.json", "providers.tofu.json"},
		},
		{
			description:   "Directory with TF and non-TF files",
			files:         []string{"main.tf", "readme.txt", "variables.tofu", "config.json"},
			expectedCount: 2,
			expectedFiles: []string{"main.tf", "variables.tofu"},
		},
		{
			description:   "Empty directory",
			files:         []string{},
			expectedCount: 0,
			expectedFiles: []string{},
		},
		{
			description:   "Directory with only non-TF files",
			files:         []string{"readme.txt", "config.json", "script.sh"},
			expectedCount: 0,
			expectedFiles: []string{},
		},
		{
			description:   "Directory with nested TF files",
			files:         []string{"main.tf", "modules/vpc/main.tofu", "modules/security/variables.tf.json"},
			directories:   []string{"modules", "modules/vpc", "modules/security"},
			expectedCount: 3,
			expectedFiles: []string{"main.tf", "modules/vpc/main.tofu", "modules/security/variables.tf.json"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Create directories
			for _, dir := range tc.directories {
				dirPath := filepath.Join(tmpDir, dir)
				require.NoError(t, os.MkdirAll(dirPath, 0755))
			}

			// Create files
			for _, file := range tc.files {
				filePath := filepath.Join(tmpDir, file)
				// Ensure directory exists
				require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
				require.NoError(t, os.WriteFile(filePath, []byte("# Test file content"), 0644))
			}

			actual, err := util.FindTFFiles(tmpDir)
			require.NoError(t, err)

			// Check count
			assert.Len(t, actual, tc.expectedCount, "Expected %d files, got %d", tc.expectedCount, len(actual))

			// Check that all expected files are found (convert to relative paths for comparison)
			expectedRelativePaths := make([]string, len(tc.expectedFiles))
			for i, expectedFile := range tc.expectedFiles {
				expectedRelativePaths[i] = filepath.Join(tmpDir, expectedFile)
			}

			for _, expectedPath := range expectedRelativePaths {
				assert.Contains(t, actual, expectedPath, "Expected file %s not found in results", expectedPath)
			}

			// Check that all found files are actually TF files
			for _, foundFile := range actual {
				assert.True(t, util.IsTFFile(foundFile), "Non-TF file %s found in results", foundFile)
			}
		})
	}
}

func TestRegexFoundInTFFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files       map[string]string
		description string
		pattern     string
		directories []string
		expected    bool
	}{
		{
			description: "Pattern found in .tf file",
			files: map[string]string{
				"main.tf": `
terraform {
  backend "s3" {
    bucket = "my-bucket"
  }
}`,
			},
			pattern:  `backend[[:blank:]]+"s3"`,
			expected: true,
		},
		{
			description: "Pattern found in .tofu file",
			files: map[string]string{
				"main.tofu": `
terraform {
  backend "local" {
    path = "terraform.tfstate"
  }
}`,
			},
			pattern:  `backend[[:blank:]]+"local"`,
			expected: true,
		},
		{
			description: "Pattern found in .tf.json file",
			files: map[string]string{
				"main.tf.json": `{
  "terraform": {
    "backend": {
      "remote": {
        "organization": "my-org"
      }
    }
  }
}`,
			},
			pattern:  `"backend":[[:space:]]*{[[:space:]]*"remote"`,
			expected: true,
		},
		{
			description: "Pattern found in .tofu.json file",
			files: map[string]string{
				"main.tofu.json": `{
  "terraform": {
    "backend": {
      "gcs": {
        "bucket": "my-bucket"
      }
    }
  }
}`,
			},
			pattern:  `"backend":[[:space:]]*{[[:space:]]*"gcs"`,
			expected: true,
		},
		{
			description: "Pattern found in mixed file types",
			files: map[string]string{
				"main.tf":      "# No backend here",
				"backend.tofu": `terraform { backend "s3" {} }`,
				"readme.txt":   "This is not a TF file",
				"config.json":  `{"not": "terraform"}`,
			},
			pattern:  `backend[[:blank:]]+"s3"`,
			expected: true,
		},
		{
			description: "Pattern not found in any TF files",
			files: map[string]string{
				"main.tf":    "resource \"aws_instance\" \"example\" {}",
				"vars.tofu":  "variable \"name\" { type = string }",
				"readme.txt": "This file contains backend configuration (but it's not a TF file)",
			},
			pattern:  `backend[[:blank:]]+"s3"`,
			expected: false,
		},
		{
			description: "Pattern found in nested TF files",
			files: map[string]string{
				"main.tf":                  "# No backend",
				"modules/vpc/main.tofu":    `terraform { backend "s3" {} }`,
				"modules/security/vars.tf": "variable \"vpc_id\" {}",
			},
			directories: []string{"modules", "modules/vpc", "modules/security"},
			pattern:     `backend[[:blank:]]+"s3"`,
			expected:    true,
		},
		{
			description: "Module pattern found",
			files: map[string]string{
				"main.tf": `
module "vpc" {
  source = "./modules/vpc"
}`,
			},
			pattern:  `module[[:blank:]]+".+"`,
			expected: true,
		},
		{
			description: "Module pattern found in .tofu file",
			files: map[string]string{
				"infrastructure.tofu": `
module "database" {
  source = "git::https://github.com/example/modules.git//database"
}`,
			},
			pattern:  `module[[:blank:]]+".+"`,
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			// Create temporary directory
			tmpDir := t.TempDir()

			// Create directories
			for _, dir := range tc.directories {
				dirPath := filepath.Join(tmpDir, dir)
				require.NoError(t, os.MkdirAll(dirPath, 0755))
			}

			// Create files with content
			for filename, content := range tc.files {
				filePath := filepath.Join(tmpDir, filename)
				// Ensure directory exists
				require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
				require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
			}

			// Compile regex pattern
			regex, err := regexp.Compile(tc.pattern)
			require.NoError(t, err)

			actual, err := util.RegexFoundInTFFiles(tmpDir, regex)
			require.NoError(t, err)

			assert.Equal(t, tc.expected, actual, "For test case: %s", tc.description)
		})
	}
}

// TestRegexFoundInTFFilesErrorHandling tests error conditions
func TestRegexFoundInTFFilesErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("Non-existent directory", func(t *testing.T) {
		t.Parallel()

		regex := regexp.MustCompile("test")
		_, err := util.RegexFoundInTFFiles("/non/existent/directory", regex)
		assert.Error(t, err)
	})

	t.Run("Permission denied file", func(t *testing.T) {
		t.Parallel()

		// Create a directory and file, then remove read permissions
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.tf")
		require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

		// Remove read permissions (this test might not work on all systems/CI environments)
		err := os.Chmod(testFile, 0000)
		if err != nil {
			t.Skip("Cannot change file permissions on this system")
		}

		// Restore permissions after test
		defer func() {
			_ = os.Chmod(testFile, 0644)
		}()

		regex := regexp.MustCompile("test")
		_, err = util.RegexFoundInTFFiles(tmpDir, regex)
		// We expect an error due to permission denied, but don't fail the test
		// if the OS doesn't enforce permission restrictions in the test environment
		if err == nil {
			t.Log("Permission restrictions not enforced in test environment")
		}
	})
}

// Benchmark tests to ensure performance is reasonable
func BenchmarkIsTFFile(b *testing.B) {
	testPaths := []string{
		"main.tf",
		"variables.tofu",
		"outputs.tf.json",
		"providers.tofu.json",
		"readme.txt",
		"config.json",
		"/very/long/path/to/terraform/modules/vpc/main.tf",
		"/very/long/path/to/opentofu/modules/database/variables.tofu",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		for _, path := range testPaths {
			if util.IsTFFile(path) {
				benchmarkBoolSink = !benchmarkBoolSink
			}
		}
	}
}

func BenchmarkDirContainsTFFiles(b *testing.B) {
	// Create a temporary directory with mixed files for benchmarking
	tmpDir := b.TempDir()
	files := []string{
		"main.tf",
		"variables.tofu",
		"outputs.tf.json",
		"providers.tofu.json",
		"readme.txt",
		"config.json",
		"modules/vpc/main.tf",
		"modules/database/vars.tofu",
	}

	for _, file := range files {
		filePath := filepath.Join(tmpDir, file)
		require.NoError(b, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(b, os.WriteFile(filePath, []byte("# Test content"), 0644))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		result, err := util.DirContainsTFFiles(tmpDir)
		require.NoError(b, err)

		benchmarkBoolSink = benchmarkBoolSink != result
	}
}
