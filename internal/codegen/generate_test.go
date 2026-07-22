package codegen_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteStateConfigToTerraformCode(t *testing.T) {
	t.Parallel()

	expectedOrdered := []byte(`terraform {
  backend "ordered" {
    a = 1
    b = 2
    c = 3
  }
  encryption {
    key_provider "test" "default" {
      a = 1
      b = 2
      c = 3
    }
    method "aes_gcm" "default" {
      keys = key_provider.test.default
    }
    state {
      method = method.aes_gcm.default
    }
    plan {
      method = method.aes_gcm.default
    }
  }
}
`)
	expectedEmptyConfig := []byte(`terraform {
  backend "empty" {
  }
  encryption {
    key_provider "test" "default" {
    }
    method "aes_gcm" "default" {
      keys = key_provider.test.default
    }
    state {
      method = method.aes_gcm.default
    }
    plan {
      method = method.aes_gcm.default
    }
  }
}
`)
	expectedEmptyEncryption := []byte(`terraform {
  backend "empty" {
  }
}
`)
	expectedS3WithAssumeRole := []byte(`terraform {
  backend "s3" {
    assume_role = {
      duration        = "1h30m"
      external_id     = "123456789012"
      policy          = "{}"
      policy_arns     = ["arn:aws:iam::123456789012:policy/MyPolicy", "arn:aws:iam::123456789012:policy/MyOtherPolicy"]
      role_arn        = "arn:aws:iam::123456789012:role/MyRole"
      session_name    = "MySession"
      source_identity = "123456789012"
      tags = {
        key = "value"
      }
      transitive_tag_keys = ["key", "another-key"]
    }
    bucket = "mybucket"
  }
}
`)
	expectedS3WithAssumeRoleWithWebIdentity := []byte(`terraform {
  backend "s3" {
    assume_role_with_web_identity = {
      duration                = "1h30m"
      policy                  = "{}"
      policy_arns             = ["arn:aws:iam::123456789012:policy/MyPolicy"]
      role_arn                = "arn:aws:iam::123456789012:role/MyRole"
      session_name            = "MySession"
      web_identity_token      = "123456789012"
      web_identity_token_file = "/path/to/web_identity_token_file"
    }
    bucket = "mybucket"
  }
}
`)

	testCases := []struct {
		name       string
		backend    string
		config     map[string]any
		encryption map[string]any
		expected   []byte
		expectErr  bool
	}{
		{
			"remote-state-config-unsorted-keys",
			"ordered",
			map[string]any{
				"b": 2,
				"a": 1,
				"c": 3,
			},
			map[string]any{
				"key_provider": "test",
				"b":            2,
				"a":            1,
				"c":            3,
			},
			expectedOrdered,
			false,
		},
		{
			"remote-state-config-empty",
			"empty",
			map[string]any{},
			map[string]any{
				"key_provider": "test",
			},
			expectedEmptyConfig,
			false,
		},
		{
			"remote-state-encryption-empty",
			"empty",
			map[string]any{},
			map[string]any{},
			expectedEmptyEncryption,
			false,
		},
		{
			"remote-state-encryption-missing-key-provider",
			"empty",
			map[string]any{},
			map[string]any{
				"a": 1,
			},
			[]byte(""),
			true,
		},
		{
			"s3-backend-with-assume-role",
			"s3",
			map[string]any{
				"bucket": "mybucket",
				"assume_role": "{role_arn=\"arn:aws:iam::123456789012:role/MyRole\"," +
					"tags={key=\"value\"}, duration=\"1h30m\", " +
					"external_id=\"123456789012\", policy=\"{}\", " +
					"policy_arns=[\"arn:aws:iam::123456789012:policy/MyPolicy\"," +
					"\"arn:aws:iam::123456789012:policy/MyOtherPolicy\"], " +
					"session_name=\"MySession\", " +
					"source_identity=\"123456789012\", " +
					"transitive_tag_keys=[\"key\",\"another-key\"]}",
			},
			map[string]any{},
			expectedS3WithAssumeRole,
			false,
		},
		{
			"s3-backend-with-assume-role-with-web-identity",
			"s3",
			map[string]any{
				"bucket": "mybucket",
				"assume_role_with_web_identity": "{role_arn=" +
					"\"arn:aws:iam::123456789012:role/MyRole\"," +
					"duration=\"1h30m\", policy=\"{}\", " +
					"policy_arns=[\"arn:aws:iam::123456789012" +
					":policy/MyPolicy\"], " +
					"session_name=\"MySession\", " +
					"web_identity_token=\"123456789012\", " +
					"web_identity_token_file=" +
					"\"/path/to/web_identity_token_file\"}",
			},
			map[string]any{},
			expectedS3WithAssumeRoleWithWebIdentity,
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			output, err := codegen.RemoteStateConfigToTerraformCode(
				tc.backend,
				tc.config,
				tc.encryption,
			)
			// validates the first output.
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, bytes.Contains(output, []byte(tc.backend)))
				// Comparing as string produces a nicer diff
				assert.Equal(t, string(tc.expected), string(output))
			}

			// runs the function a few of times again. All the outputs must be
			// equal to the first output.
			for range 20 {
				actual, _ := codegen.RemoteStateConfigToTerraformCode(
					tc.backend,
					tc.config,
					tc.encryption,
				)
				assert.Equal(t, output, actual)
			}
		})
	}
}

// TestRemoteStateConfigToTerraformCode_BoolValues verifies that native bool
// values in the config map produce unquoted true/false in the generated HCL.
// This is the expected output when string booleans from HCL ternary type
// unification are normalized back to Go bools before reaching codegen.
func TestRemoteStateConfigToTerraformCode_BoolValues(t *testing.T) {
	t.Parallel()

	expected := []byte(`terraform {
  backend "s3" {
    bucket       = "my-bucket"
    encrypt      = true
    key          = "terraform.tfstate"
    region       = "us-east-1"
    use_lockfile = true
  }
}
`)

	config := map[string]any{
		"bucket":       "my-bucket",
		"key":          "terraform.tfstate",
		"region":       "us-east-1",
		"encrypt":      true,
		"use_lockfile": true,
	}

	output, err := codegen.RemoteStateConfigToTerraformCode("s3", config, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, string(expected), string(output))
}

// TestRemoteStateConfigToTerraformCode_StringBoolProducesQuotedValue demonstrates
// that string "true"/"false" values produce quoted string literals in generated HCL.
// The fix for #5646 normalizes these in S3 GetTFInitArgs before they reach codegen.
func TestRemoteStateConfigToTerraformCode_StringBoolProducesQuotedValue(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"bucket":       "my-bucket",
		"key":          "terraform.tfstate",
		"region":       "us-east-1",
		"use_lockfile": "true",
	}

	output, err := codegen.RemoteStateConfigToTerraformCode("s3", config, map[string]any{})
	require.NoError(t, err)

	// String "true" produces a quoted string literal in HCL, which Terraform rejects
	assert.Contains(t, string(output), `use_lockfile = "true"`)
}

func TestFmtGeneratedFile(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	bTrue := true
	bFalse := false

	testCases := []struct {
		fmt      *bool
		name     string
		path     string
		contents string
		expected string
		ifExists codegen.GenerateConfigExists
		disabled bool
	}{
		{
			name:     "fmt-simple-hcl-file",
			fmt:      &bTrue,
			path:     fmt.Sprintf("%s/%s", testDir, "fmt_simple.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\" {\n  type    = string\n  default = \"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
		{
			name:     "fmt-hcl-file-by-default",
			path:     fmt.Sprintf("%s/%s", testDir, "fmt_hcl_file_by_default.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\" {\n  type    = string\n  default = \"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
		{
			name:     "ignore-hcl-fmt",
			fmt:      &bFalse,
			path:     fmt.Sprintf("%s/%s", testDir, "ignore_hcl_fmt.hcl"),
			contents: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			expected: "variable \"msg\"{\ntype=string\n  default=\"hello\"\n}\n",
			ifExists: codegen.ExistsError,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tc.path,
				IfExists:         tc.ifExists,
				CommentPrefix:    "",
				DisableSignature: true,
				Contents:         tc.contents,
				Disable:          tc.disabled,
				HclFmt:           tc.fmt,
			}

			l := logger.CreateLogger()
			err := codegen.WriteToFile(l, "", &config)
			require.NoError(t, err)

			assert.True(t, util.FileExists(tc.path))

			fileContent, err := os.ReadFile(tc.path)
			require.NoError(t, err)

			assert.Equal(t, tc.expected, string(fileContent))
		})
	}
}

func TestGenerateDisabling(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	testCases := []struct {
		name     string
		path     string
		contents string
		ifExists codegen.GenerateConfigExists
		disabled bool
	}{
		{
			name:     "generate-disabled-true",
			path:     fmt.Sprintf("%s/%s", testDir, "disabled_true"),
			contents: "this file should not be generated",
			ifExists: codegen.ExistsError,
			disabled: true,
		},
		{
			name:     "generate-disabled-false",
			path:     fmt.Sprintf("%s/%s", testDir, "disabled_false"),
			contents: "this file should be generated",
			ifExists: codegen.ExistsError,
			disabled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			config := codegen.GenerateConfig{
				Path:             tc.path,
				IfExists:         tc.ifExists,
				CommentPrefix:    "",
				DisableSignature: false,
				Contents:         tc.contents,
				Disable:          tc.disabled,
			}

			l := logger.CreateLogger()
			err := codegen.WriteToFile(l, "", &config)
			require.NoError(t, err)

			if tc.disabled {
				assert.True(t, util.FileNotExists(tc.path))
			} else {
				assert.True(t, util.FileExists(tc.path))
			}
		})
	}
}

func TestReplaceAllCommasOutsideQuotesWithNewLines(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "happy-path-basic-replacement",
			input: `key=value,another=value,third=value`,
			expected: `key=value
another=value
third=value`,
		},
		{
			name:  "comma-inside-quotes",
			input: `key="value,with,commas",another=value`,
			expected: `key="value,with,commas"
another=value`,
		},
		{
			name:  "mixed-quotes-and-commas",
			input: `key="value,with,commas",simple=value,quoted="hello,world"`,
			expected: `key="value,with,commas"
simple=value
quoted="hello,world"`,
		},
		{
			name:     "empty-string",
			input:    ``,
			expected: ``,
		},
		{
			name:     "no-commas",
			input:    `key=value`,
			expected: `key=value`,
		},
		{
			name:  "comma-inside-list-expression",
			input: `transitive_tag_keys=["Project","Projects"],role_arn="test-role"`,
			expected: `transitive_tag_keys=["Project","Projects"]
role_arn="test-role"`,
		},
		{
			name:  "comma-inside-object-expression",
			input: `config={env="prod",team="platform"},enabled=true`,
			expected: `config={env="prod",team="platform"}
enabled=true`,
		},
		{
			name:  "comma-inside-nested-list-and-object",
			input: `config={tags=["a","b"],env="prod"},enabled=true`,
			expected: `config={tags=["a","b"],env="prod"}
enabled=true`,
		},
		{
			name:  "mixed-quotes-lists-and-top-level-commas",
			input: `message="hello,world",tags=["a","b"],enabled=true`,
			expected: `message="hello,world"
tags=["a","b"]
enabled=true`,
		},
		{
			name:  "nested-object-containing-list",
			input: `assume_role={policy_arns=["arn:1","arn:2"],session_name="test"},region="us-east-1"`,
			expected: `assume_role={policy_arns=["arn:1","arn:2"],session_name="test"}
region="us-east-1"`,
		},
		{
			name:  "comma-inside-function-call",
			input: `tags=merge({a="1"},{b="2"}),enabled=true`,
			expected: `tags=merge({a="1"},{b="2"})
enabled=true`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := codegen.ReplaceAllCommasOutsideQuotesWithNewLines(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

// TestWriteToFileOverwritesReadOnlyTarget verifies that overwrite modes replace
// an existing target even when CAS materialized it as a read-only file.
func TestWriteToFileOverwritesReadOnlyTarget(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("read-only permission bits are not meaningfully observable on Windows")
	}

	testDir := helpers.TmpDirWOSymlinks(t)

	existingBody := "terraform {\n  required_version = \">= 1.0.0\"\n}\n"
	generatedBody := "terraform {\n  required_version = \">= 1.3.0\"\n}\n"

	testCases := []struct {
		name             string
		existingContents string
		contents         string
		commentPrefix    string
		ifExists         codegen.GenerateConfigExists
		existingPerms    os.FileMode
		disableSignature bool
	}{
		{
			name:             "overwrite-read-only-existing-file",
			existingContents: existingBody,
			contents:         generatedBody,
			ifExists:         codegen.ExistsOverwrite,
			existingPerms:    0444,
			disableSignature: true,
		},
		{
			name:             "overwrite-writable-existing-file",
			existingContents: existingBody,
			contents:         generatedBody,
			ifExists:         codegen.ExistsOverwrite,
			existingPerms:    0644,
			disableSignature: true,
		},
		{
			name:             "overwrite-terragrunt-read-only-generated-file",
			existingContents: codegen.DefaultCommentPrefix + codegen.TerragruntGeneratedSignature + "\n" + existingBody,
			contents:         generatedBody,
			commentPrefix:    codegen.DefaultCommentPrefix,
			ifExists:         codegen.ExistsOverwriteTerragrunt,
			existingPerms:    0444,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(testDir, tc.name+".tf")
			writeFileWithPerms(t, path, tc.existingContents, tc.existingPerms)

			config := codegen.GenerateConfig{
				Path:             path,
				IfExists:         tc.ifExists,
				CommentPrefix:    tc.commentPrefix,
				DisableSignature: tc.disableSignature,
				Contents:         tc.contents,
			}

			l := logger.CreateLogger()
			require.NoError(t, codegen.WriteToFile(l, "", &config))

			fileContent, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(fileContent), tc.contents)
			assert.NotContains(t, string(fileContent), ">= 1.0.0")

			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.NotZero(t, info.Mode().Perm()&0200, "regenerated file must be owner-writable")
		})
	}
}

// TestWriteToFileSkipAndErrorLeaveExistingFileIntact verifies that
// non-overwrite modes never remove or modify a pre-existing target.
func TestWriteToFileSkipAndErrorLeaveExistingFileIntact(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	existingBody := "terraform {\n  required_version = \">= 1.0.0\"\n}\n"

	testCases := []struct {
		name     string
		ifExists codegen.GenerateConfigExists
		wantErr  bool
	}{
		{
			name:     "skip-leaves-existing-file",
			ifExists: codegen.ExistsSkip,
		},
		{
			name:     "error-leaves-existing-file",
			ifExists: codegen.ExistsError,
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(testDir, tc.name+".tf")
			writeFileWithPerms(t, path, existingBody, 0444)

			config := codegen.GenerateConfig{
				Path:             path,
				IfExists:         tc.ifExists,
				DisableSignature: true,
				Contents:         "terraform {}\n",
			}

			l := logger.CreateLogger()
			writeErr := codegen.WriteToFile(l, "", &config)

			if tc.wantErr {
				var existsErr codegen.GenerateFileExistsError

				require.ErrorAs(t, writeErr, &existsErr)
			}

			if !tc.wantErr {
				require.NoError(t, writeErr)
			}

			fileContent, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, existingBody, string(fileContent), "existing file must stay intact")
		})
	}
}

// TestWriteToFileOverwriteDoesNotMutateHardlinkedStore verifies that
// overwriting a target hard-linked into the CAS store breaks the link instead
// of mutating the shared blob.
func TestWriteToFileOverwriteDoesNotMutateHardlinkedStore(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("read-only permission bits are not meaningfully observable on Windows")
	}

	testDir := helpers.TmpDirWOSymlinks(t)

	storePath := filepath.Join(testDir, "store-blob")
	targetPath := filepath.Join(testDir, "versions.tf")
	storeContents := "terraform {\n  required_version = \">= 1.0.0\"\n}\n"

	writeFileWithPerms(t, storePath, storeContents, 0444)
	require.NoError(t, os.Link(storePath, targetPath))

	storeInfoBefore, err := os.Stat(storePath)
	require.NoError(t, err)

	config := codegen.GenerateConfig{
		Path:             targetPath,
		IfExists:         codegen.ExistsOverwrite,
		DisableSignature: true,
		Contents:         "terraform {\n  required_version = \">= 1.3.0\"\n}\n",
	}

	l := logger.CreateLogger()
	require.NoError(t, codegen.WriteToFile(l, "", &config))

	storeContentAfter, err := os.ReadFile(storePath)
	require.NoError(t, err)
	assert.Equal(t, storeContents, string(storeContentAfter), "store blob content must stay intact")

	storeInfoAfter, err := os.Stat(storePath)
	require.NoError(t, err)
	assert.Equal(
		t,
		os.FileMode(0444),
		storeInfoAfter.Mode().Perm(),
		"store blob must stay read-only",
	)

	targetInfo, err := os.Stat(targetPath)
	require.NoError(t, err)
	assert.False(t, os.SameFile(storeInfoBefore, targetInfo),
		"target must get a fresh inode instead of sharing the store blob")

	targetContent, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Contains(t, string(targetContent), ">= 1.3.0")
}

// TestWriteToFileTargetIsDirectory verifies that a directory at the target
// path still produces an error instead of being removed.
func TestWriteToFileTargetIsDirectory(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	targetPath := filepath.Join(testDir, "versions.tf")
	require.NoError(t, os.Mkdir(targetPath, 0755))

	config := codegen.GenerateConfig{
		Path:             targetPath,
		IfExists:         codegen.ExistsOverwrite,
		DisableSignature: true,
		Contents:         "terraform {}\n",
	}

	l := logger.CreateLogger()
	require.Error(t, codegen.WriteToFile(l, "", &config))
	assert.DirExists(t, targetPath)
}

// TestWriteToFileDisabledRemovesReadOnlyFile verifies that the if_disabled
// remove path handles a read-only target.
func TestWriteToFileDisabledRemovesReadOnlyFile(t *testing.T) {
	t.Parallel()

	testDir := helpers.TmpDirWOSymlinks(t)

	targetPath := filepath.Join(testDir, "versions.tf")
	writeFileWithPerms(t, targetPath, "terraform {}\n", 0444)

	config := codegen.GenerateConfig{
		Path:       targetPath,
		IfExists:   codegen.ExistsOverwrite,
		Contents:   "terraform {}\n",
		Disable:    true,
		IfDisabled: codegen.DisabledRemove,
	}

	l := logger.CreateLogger()
	require.NoError(t, codegen.WriteToFile(l, "", &config))
	assert.True(t, util.FileNotExists(targetPath))
}

// writeFileWithPerms writes contents first and tightens permissions afterwards
// so read-only fixtures still receive their contents.
func writeFileWithPerms(t *testing.T, path, contents string, perms os.FileMode) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
	require.NoError(t, os.Chmod(path, perms))
}
