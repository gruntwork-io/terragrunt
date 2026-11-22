package tf_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	fingerprintRootTF = `terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "~> 3.2"
    }
  }
}

module "example" {
  source = "./example"
}

module "outside" {
  source = "../modules/outside"
}

module "remote" {
  source = "git::https://example.com/remote.git"
}
`
	fingerprintChildTF = `output "ok" {
  value = true
}
`
	fingerprintLock = `provider "registry.opentofu.org/hashicorp/null" {
  version = "3.2.4"
}
`
	fingerprintManifest = `{"Modules":[` +
		`{"Key":"","Source":"","Dir":"."},` +
		`{"Key":"example","Source":"./example","Dir":"example"},` +
		`{"Key":"outside","Source":"../modules/outside","Dir":"../modules/outside"},` +
		`{"Key":"remote","Source":"git::https://example.com/remote.git","Dir":".terraform/modules/remote"}` +
		`]}`
	fingerprintGeneratedTF = `locals {
  generated_at = "2026-01-01T00:00:00Z"
}

provider "null" {
  alias   = "tagged"
  version = "~> 3.2"

  default_tags {
    tags = { Commit = "0123abc" }
  }
}

variable "count" {
  default = 1
}

resource "null_resource" "this" {
  provider = null

  triggers = {
    note = "before"
  }
}

module "registry" {
  source  = "registry.opentofu.org/example/module/null"
  version = "1.0.0"
}

output "generated_at" {
  value = local.generated_at
}
`
	fingerprintRandomTF = `
terraform {
  required_providers {
    random = {
      source = "registry.opentofu.org/hashicorp/random"
    }
  }
}
`
	fingerprintBackendTF = `terraform {
  backend "s3" {
    bucket = "state"
    key    = "unit/tofu.tfstate"
  }
}
`
	fingerprintBrokenTF = `resource "null_resource" "this" {
`
)

// writeInitializedUnit lays out under base an initialized unit at base/unit that calls a module
// inside the unit, one outside it, and one init downloaded, and returns the unit directory.
func writeInitializedUnit(t *testing.T, fsys vfs.FS, base string) string {
	t.Helper()

	unit := filepath.Join(base, "unit")
	files := map[string]string{
		filepath.Join(unit, "main.tf"):                                          fingerprintRootTF,
		filepath.Join(unit, "generated.tf"):                                     fingerprintGeneratedTF,
		filepath.Join(unit, "backend.tf"):                                       fingerprintBackendTF,
		filepath.Join(unit, "README.md"):                                        "readme",
		filepath.Join(unit, "examples", "basic", "main.tf"):                     `module "example" { source = "../.." }`,
		filepath.Join(unit, "example", "main.tf"):                               fingerprintChildTF,
		filepath.Join(base, "modules", "outside", "main.tf"):                    fingerprintChildTF,
		filepath.Join(unit, tf.TerraformLockFile):                               fingerprintLock,
		filepath.Join(unit, ".terraform", "modules", "modules.json"):            fingerprintManifest,
		filepath.Join(unit, ".terraform", "modules", "remote", "main.tf"):       fingerprintChildTF,
		filepath.Join(unit, ".terraform", "providers", "placeholder"):           "",
		filepath.Join(unit, ".terraform", "providers", "nested", "placeholder"): "",
	}

	for path, content := range files {
		require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	return unit
}

func TestInitFingerprintChangesOnlyWhenInitWould(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		mutate      func(t *testing.T, fsys vfs.FS, unit string)
		name        string
		wantChanged bool
	}{
		{
			name:        "nothing changes",
			mutate:      func(_ *testing.T, _ vfs.FS, _ string) {},
			wantChanged: false,
		},
		{
			name: "comment added to a root module file",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), fingerprintRootTF+"\n# edited\n")
			},
			wantChanged: false,
		},
		{
			name: "provider version constraint changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintRootTF, `"~> 3.2"`, `"~> 3.1"`, 1)
				writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "provider requirement added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), fingerprintGeneratedTF+fingerprintRandomTF)
			},
			wantChanged: true,
		},
		{
			name: "resource of a provider with no requirement added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := fingerprintGeneratedTF + `resource "random_pet" "this" {}` + "\n"
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "data source of a provider with no requirement added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := fingerprintGeneratedTF + `data "http" "this" { url = "https://example.com" }` + "\n"
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "provider meta-argument of a resource changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, "provider = null", "provider = other", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "provider block of another provider added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := fingerprintGeneratedTF + `provider "random" {}` + "\n"
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "deprecated provider version constraint changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, `"~> 3.2"`, `"~> 3.1"`, 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "provider alias changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, `"tagged"`, `"other"`, 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "reserved provider meta-argument added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, `alias   = "tagged"`, `alias = "tagged"
  count = 2`, 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "generated locals changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: false,
		},
		{
			name: "generated provider arguments changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, "0123abc", "4567def", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: false,
		},
		{
			name: "resource arguments, variables, and outputs changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.NewReplacer(
					`"before"`, `"after"`,
					`default = 1`, `default = 2`,
					`value = local.generated_at`, `value = "after"`,
				).Replace(fingerprintGeneratedTF)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: false,
		},
		{
			name: "module source changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintRootTF, "remote.git", "remote.git?ref=v2", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "module version changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintGeneratedTF, `"1.0.0"`, `"1.1.0"`, 1)
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "generated backend configuration changed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintBackendTF, "unit/tofu.tfstate", "other/tofu.tfstate", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "backend.tf"), edited)
			},
			wantChanged: true,
		},
		{
			name: "file stops parsing",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), fingerprintGeneratedTF+"locals {\n")
			},
			wantChanged: true,
		},
		{
			name: "root module file added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "extra.tf"), fingerprintChildTF)
			},
			wantChanged: true,
		},
		{
			name: "root module .tf.json file added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "extra.tf.json"), `{"output": {"x": {"value": 1}}}`)
			},
			wantChanged: true,
		},
		{
			name: "root module file removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				require.NoError(t, fsys.Remove(filepath.Join(unit, "main.tf")))
			},
			wantChanged: true,
		},
		{
			name: "module inside the unit requires another provider",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "example", "main.tf"), fingerprintChildTF+fingerprintRandomTF)
			},
			wantChanged: true,
		},
		{
			name: "output of a module inside the unit edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				edited := strings.Replace(fingerprintChildTF, "true", "false", 1)
				writeTestFile(t, fsys, filepath.Join(unit, "example", "main.tf"), edited)
			},
			wantChanged: false,
		},
		{
			name: "module outside the unit requires another provider",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				outside := filepath.Join(unit, "..", "modules", "outside", "main.tf")
				writeTestFile(t, fsys, outside, fingerprintChildTF+fingerprintRandomTF)
			},
			wantChanged: true,
		},
		{
			name: "module outside the unit calls a module",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				outside := filepath.Join(unit, "..", "modules", "outside", "main.tf")
				writeTestFile(t, fsys, outside, fingerprintChildTF+`module "nested" { source = "./nested" }`+"\n")
			},
			wantChanged: true,
		},
		{
			name: "module outside the unit removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				require.NoError(t, fsys.RemoveAll(filepath.Join(unit, "..", "modules", "outside")))
			},
			wantChanged: true,
		},
		{
			name: "lock file edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, tf.TerraformLockFile), fingerprintLock+"\n# edited\n")
			},
			wantChanged: true,
		},
		{
			name: "lock file removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				require.NoError(t, fsys.Remove(filepath.Join(unit, tf.TerraformLockFile)))
			},
			wantChanged: true,
		},
		{
			name: "providers directory removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				require.NoError(t, fsys.RemoveAll(filepath.Join(unit, ".terraform", "providers")))
			},
			wantChanged: true,
		},
		{
			name: "modules directory removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				require.NoError(t, fsys.RemoveAll(filepath.Join(unit, ".terraform", "modules")))
			},
			wantChanged: true,
		},
		{
			name: "downloaded module edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, ".terraform", "modules", "remote", "main.tf"), "# edited")
			},
			wantChanged: false,
		},
		{
			name: "file in a subdirectory the root module never loads edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "examples", "basic", "main.tf"), "# edited")
			},
			wantChanged: false,
		},
		{
			name: "file that is not configuration edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "README.md"), "edited")
			},
			wantChanged: false,
		},
		{
			name: "provider package added under the providers directory",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, ".terraform", "providers", "added"), "")
			},
			wantChanged: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			unit := writeInitializedUnit(t, fsys, "/work")
			before := fingerprint(t, fsys, unit)

			tc.mutate(t, fsys, unit)

			assert.Equal(t, tc.wantChanged, !current(t, fsys, unit, before))
		})
	}
}

func TestInitFingerprintCountsEveryByteWhenInitMayDependOnThem(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		others map[string]string
		name   string
		file   string
		before string
		after  string
	}{
		{
			name:   "variable default that a module source in a .tf.json file refers to changed",
			others: map[string]string{"static.tf.json": `{"module": {"static": {"source": "${var.src}"}}}`},
			file:   "vars.tf",
			before: "variable \"src\" {\n  default = \"./a\"\n}\n",
			after:  "variable \"src\" {\n  default = \"./b\"\n}\n",
		},
		{
			name:   "variable default changed next to a file that fails to parse",
			others: map[string]string{"broken.tf": fingerprintBrokenTF},
			file:   "vars.tf",
			before: "variable \"src\" {\n  default = \"./a\"\n}\n",
			after:  "variable \"src\" {\n  default = \"./b\"\n}\n",
		},
		{
			name:   "variable default changed next to a file larger than 1 MiB",
			others: map[string]string{"large.tf": strings.Repeat("# padding\n", 110_000)},
			file:   "vars.tf",
			before: "variable \"src\" {\n  default = \"./a\"\n}\n",
			after:  "variable \"src\" {\n  default = \"./b\"\n}\n",
		},
		{
			name:   "local that a module source refers to changed",
			file:   "static.tf",
			before: "locals {\n  dir = \"./example\"\n}\n\nmodule \"static\" {\n  source = local.dir\n}\n",
			after:  "locals {\n  dir = \"./other\"\n}\n\nmodule \"static\" {\n  source = local.dir\n}\n",
		},
		{
			name:   "variable default that a module version refers to changed",
			file:   "static.tf",
			before: "variable \"v\" {\n  default = \"1.0.0\"\n}\n\nmodule \"static\" {\n  version = var.v\n}\n",
			after:  "variable \"v\" {\n  default = \"2.0.0\"\n}\n\nmodule \"static\" {\n  version = var.v\n}\n",
		},
		{
			name:   "variable that a backend refers to changed",
			file:   "static.tf",
			before: "variable \"key\" {\n  default = \"a\"\n}\n\nterraform {\n  backend \"s3\" {\n    key = var.key\n  }\n}\n",
			after:  "variable \"key\" {\n  default = \"b\"\n}\n\nterraform {\n  backend \"s3\" {\n    key = var.key\n  }\n}\n",
		},
		{
			name:   "local that a provider for_each refers to changed",
			file:   "static.tf",
			before: "locals {\n  regions = [\"a\"]\n}\n\nprovider \"null\" {\n  for_each = toset(local.regions)\n}\n",
			after:  "locals {\n  regions = [\"b\"]\n}\n\nprovider \"null\" {\n  for_each = toset(local.regions)\n}\n",
		},
		{
			name:   "file that fails to parse edited",
			file:   "broken.tf",
			before: fingerprintBrokenTF,
			after:  fingerprintBrokenTF + "\n# edited\n",
		},
		{
			name:   "locals of a .tf.json file changed",
			file:   "meta.tf.json",
			before: `{"locals": {"at": "before"}}`,
			after:  `{"locals": {"at": "after"}}`,
		},
		{
			name:   "comment in a file larger than 1 MiB edited",
			file:   "large.tf",
			before: strings.Repeat("# padding\n", 110_000),
			after:  strings.Repeat("# padding\n", 110_000) + "# edited\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			unit := writeInitializedUnit(t, fsys, "/work")

			for name, content := range tc.others {
				writeTestFile(t, fsys, filepath.Join(unit, name), content)
			}

			writeTestFile(t, fsys, filepath.Join(unit, tc.file), tc.before)
			before := fingerprint(t, fsys, unit)

			writeTestFile(t, fsys, filepath.Join(unit, tc.file), tc.after)

			assert.False(t, current(t, fsys, unit, before))
		})
	}
}

func TestInitFingerprintCountsInputsWhenInitEvaluatesVariables(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		mutate      func(t *testing.T, fsys vfs.FS, unit string, inputs *tf.InitInputs)
		name        string
		wantChanged bool
	}{
		{
			name:        "nothing changes",
			mutate:      func(_ *testing.T, _ vfs.FS, _ string, _ *tf.InitInputs) {},
			wantChanged: false,
		},
		{
			name: "TF_VAR_ variable changed",
			mutate: func(_ *testing.T, _ vfs.FS, _ string, inputs *tf.InitInputs) {
				inputs.Env["TF_VAR_src"] = "./b"
			},
			wantChanged: true,
		},
		{
			name: "TF_CLI_ARGS_init set",
			mutate: func(_ *testing.T, _ vfs.FS, _ string, inputs *tf.InitInputs) {
				inputs.Env[tf.EnvNameTFCLIArgsInit] = "-var=src=./b"
			},
			wantChanged: true,
		},
		{
			name: "-var argument added",
			mutate: func(_ *testing.T, _ vfs.FS, _ string, inputs *tf.InitInputs) {
				inputs.Args = append(inputs.Args, "-var", "src=./b")
			},
			wantChanged: true,
		},
		{
			name: "file a -var-file argument names edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string, _ *tf.InitInputs) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "init.tfvars"), `src = "./b"`)
			},
			wantChanged: true,
		},
		{
			name: "terraform.tfvars edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string, _ *tf.InitInputs) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "terraform.tfvars"), `src = "./b"`)
			},
			wantChanged: true,
		},
		{
			name: ".auto.tfvars.json file added",
			mutate: func(t *testing.T, fsys vfs.FS, unit string, _ *tf.InitInputs) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "extra.auto.tfvars.json"), `{"src": "./b"}`)
			},
			wantChanged: true,
		},
		{
			name: "values in generated var files changed",
			mutate: func(_ *testing.T, _ vfs.FS, _ string, inputs *tf.InitInputs) {
				inputs.Generated = []byte(`{"src": "./b"}`)
			},
			wantChanged: true,
		},
		{
			name: "generated var file removed",
			mutate: func(t *testing.T, fsys vfs.FS, unit string, _ *tf.InitInputs) {
				t.Helper()
				require.NoError(t, fsys.Remove(filepath.Join(unit, "generated.auto.tfvars")))
			},
			wantChanged: false,
		},
		{
			name: "var file nothing loads edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string, _ *tf.InitInputs) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "unused.tfvars"), `src = "./b"`)
			},
			wantChanged: false,
		},
		{
			name: "unrelated environment variable changed",
			mutate: func(_ *testing.T, _ vfs.FS, _ string, inputs *tf.InitInputs) {
				inputs.Env["HOME"] = "/elsewhere"
			},
			wantChanged: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			unit := writeInitializedUnit(t, fsys, "/work")
			dataDir := filepath.Join(unit, ".terraform")

			for name, content := range map[string]string{
				"static.tf":             "variable \"src\" {}\n\nmodule \"static\" {\n  source = var.src\n}\n",
				"init.tfvars":           `src = "./a"`,
				"terraform.tfvars":      `src = "./a"`,
				"unused.tfvars":         `src = "./a"`,
				"generated.auto.tfvars": `src = "./a"`,
			} {
				writeTestFile(t, fsys, filepath.Join(unit, name), content)
			}

			inputs := tf.InitInputs{
				Env:               map[string]string{"TF_VAR_src": "./a", "HOME": "/home"},
				Args:              []string{"-var-file=init.tfvars"},
				Generated:         []byte(`{"src": "./a"}`),
				GeneratedVarFiles: []string{"generated.auto.tfvars"},
			}

			before, err := tf.ComputeInitFingerprint(fsys, unit, dataDir, returnInitInputs(&inputs))
			require.NoError(t, err)
			assert.NotEmpty(t, before.Inputs)

			tc.mutate(t, fsys, unit, &inputs)

			got, err := tf.InitFingerprintCurrent(fsys, unit, dataDir, before, returnInitInputs(&inputs))
			require.NoError(t, err)
			assert.Equal(t, tc.wantChanged, !got)
		})
	}
}

func TestInitFingerprintSkipsInputsWithoutStaticReferences(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	unit := writeInitializedUnit(t, fsys, "/work")
	dataDir := filepath.Join(unit, ".terraform")
	calls := 0

	inputs := func() (tf.InitInputs, error) {
		calls++

		return tf.InitInputs{Env: map[string]string{"TF_VAR_src": strconv.Itoa(calls)}}, nil
	}

	recorded, err := tf.ComputeInitFingerprint(fsys, unit, dataDir, inputs)
	require.NoError(t, err)
	assert.Empty(t, recorded.Inputs)

	edited := strings.Replace(fingerprintGeneratedTF, "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z", 1)
	writeTestFile(t, fsys, filepath.Join(unit, "generated.tf"), edited)

	got, err := tf.InitFingerprintCurrent(fsys, unit, dataDir, recorded, inputs)
	require.NoError(t, err)
	assert.True(t, got)
	assert.Zero(t, calls)
}

func FuzzInitFingerprint(f *testing.F) {
	for _, seed := range []string{
		fingerprintRootTF,
		fingerprintGeneratedTF,
		fingerprintBackendTF,
		fingerprintBrokenTF,
		`resource {}`,
		`provider "a" "b" { for_each = local.x }`,
		`module "m" { source = "./${var.dir}" }`,
		`x = 1`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, content string) {
		fsys := vfs.NewMemMapFS()
		unit := writeInitializedUnit(t, fsys, "/work")
		writeTestFile(t, fsys, filepath.Join(unit, "fuzz.tf"), content)

		before := fingerprint(t, fsys, unit)

		stale := before
		stale.Files = "stale"

		assert.True(t, current(t, fsys, unit, before))
		assert.True(t, current(t, fsys, unit, stale))
	})
}

func TestInitFingerprintIgnoresWhereTheUnitLives(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	here := writeInitializedUnit(t, fsys, "/here")
	there := writeInitializedUnit(t, fsys, "/there/deeper")

	assert.Equal(t, fingerprint(t, fsys, here), fingerprint(t, fsys, there))
}

func TestInitFingerprintBeforeAnyInit(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	unit := "/work/unit"
	writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), fingerprintChildTF)

	got := fingerprint(t, fsys, unit)
	assert.NotEmpty(t, got.Files)
	assert.NotEmpty(t, got.Config)
}

func TestInitFingerprintErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing root module", func(t *testing.T) {
		t.Parallel()

		_, err := tf.ComputeInitFingerprint(
			vfs.NewMemMapFS(),
			"/missing",
			"/missing/.terraform",
			noInitInputs,
		)
		require.Error(t, err)
	})

	t.Run("unparseable modules manifest", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		unit := writeInitializedUnit(t, fsys, "/work")
		writeTestFile(t, fsys, filepath.Join(unit, ".terraform", "modules", "modules.json"), "{")

		_, err := tf.ComputeInitFingerprint(fsys, unit, filepath.Join(unit, ".terraform"), noInitInputs)
		require.Error(t, err)
	})
}

func TestWriteInitFingerprintCreatesTheDataDir(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dataDir := "/work/unit/.terraform"
	written := tf.InitFingerprint{Files: "abc", Config: "def"}

	require.NoError(t, tf.WriteInitFingerprint(fsys, dataDir, written))

	read, err := tf.ReadInitFingerprint(fsys, dataDir)
	require.NoError(t, err)
	assert.Equal(t, written, read)
}

func TestReadInitFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("nothing recorded", func(t *testing.T) {
		t.Parallel()

		_, err := tf.ReadInitFingerprint(vfs.NewMemMapFS(), "/work/unit/.terraform")
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("unknown format matches no root module", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		unit := writeInitializedUnit(t, fsys, "/work")
		dataDir := filepath.Join(unit, ".terraform")
		writeTestFile(t, fsys, filepath.Join(dataDir, tf.InitFingerprintFile), fingerprint(t, fsys, unit).Files)

		read, err := tf.ReadInitFingerprint(fsys, dataDir)
		require.NoError(t, err)
		assert.False(t, current(t, fsys, unit, read))
	})
}

// fingerprint returns the fingerprint of the unit at unit with the data directory unit/.terraform.
func fingerprint(t *testing.T, fsys vfs.FS, unit string) tf.InitFingerprint {
	t.Helper()

	got, err := tf.ComputeInitFingerprint(fsys, unit, filepath.Join(unit, ".terraform"), noInitInputs)
	require.NoError(t, err)

	return got
}

// current reports whether the unit at unit still has the fingerprint recorded.
func current(t *testing.T, fsys vfs.FS, unit string, recorded tf.InitFingerprint) bool {
	t.Helper()

	got, err := tf.InitFingerprintCurrent(
		fsys,
		unit,
		filepath.Join(unit, ".terraform"),
		recorded,
		noInitInputs,
	)
	require.NoError(t, err)

	return got
}

// returnInitInputs returns a function that returns a copy of inputs as they are now.
func returnInitInputs(inputs *tf.InitInputs) func() (tf.InitInputs, error) {
	snapshot := *inputs

	return func() (tf.InitInputs, error) {
		return snapshot, nil
	}
}

// noInitInputs returns no [tf.InitInputs].
func noInitInputs() (tf.InitInputs, error) {
	return tf.InitInputs{}, nil
}

func writeTestFile(t *testing.T, fsys vfs.FS, path, content string) {
	t.Helper()

	require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
}

// writeBenchmarkUnit lays out under base an initialized unit shaped like a typical module: a
// terraform block, generated provider and backend files, variables, outputs, many resources, and a
// local module outside the unit. It returns the unit directory.
func writeBenchmarkUnit(b *testing.B, fsys vfs.FS, base string) string {
	b.Helper()

	const (
		resourceCount = 40
		variableCount = 40
		outputCount   = 20
	)

	var main, variables, outputs strings.Builder

	for i := range resourceCount {
		fmt.Fprintf(&main, `resource "aws_s3_bucket" "b%[1]d" {
  bucket = "${var.v%[1]d}-bucket"
  tags = merge(local.tags, {
    Name  = "bucket-%[1]d"
    Index = %[1]d
  })

  lifecycle {
    ignore_changes = [tags["Updated"]]
  }
}

`, i)
	}

	main.WriteString(`module "outside" {
  source = "../modules/outside"
  name   = var.v0
}

locals {
  tags = { Team = "platform" }
}
`)

	for i := range variableCount {
		fmt.Fprintf(&variables, "variable \"v%d\" {\n  type        = string\n  default     = \"value-%d\"\n"+
			"  description = \"Variable number %d.\"\n}\n\n", i, i, i)
	}

	for i := range outputCount {
		fmt.Fprintf(&outputs, "output \"o%d\" {\n  value = aws_s3_bucket.b%d.arn\n}\n\n", i, i)
	}

	unit := filepath.Join(base, "unit")
	files := map[string]string{
		filepath.Join(unit, "main.tf"):      main.String(),
		filepath.Join(unit, "variables.tf"): variables.String(),
		filepath.Join(unit, "outputs.tf"):   outputs.String(),
		filepath.Join(unit, "versions.tf"):  fingerprintRootTF,
		filepath.Join(unit, "provider.tf"): `provider "aws" {
  region = "us-east-1"
  default_tags {
    tags = { Commit = "0123456789abcdef" }
  }
}
`,
		filepath.Join(unit, "backend.tf"): `terraform {
  backend "s3" {
    bucket = "state"
    key    = "unit/terraform.tfstate"
    region = "us-east-1"
  }
}
`,
		filepath.Join(base, "modules", "outside", "main.tf"):         fingerprintChildTF + variables.String(),
		filepath.Join(unit, tf.TerraformLockFile):                    fingerprintLock,
		filepath.Join(unit, ".terraform", "modules", "modules.json"): fingerprintManifest,
	}

	for path, content := range files {
		require.NoError(b, fsys.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(b, vfs.WriteFile(fsys, path, []byte(content), 0o644))
	}

	return unit
}

func BenchmarkInitFingerprint(b *testing.B) {
	fsys := vfs.NewOSFS()
	unit := writeBenchmarkUnit(b, fsys, b.TempDir())
	dataDir := filepath.Join(unit, ".terraform")

	recorded, err := tf.ComputeInitFingerprint(fsys, unit, dataDir, noInitInputs)
	require.NoError(b, err)

	b.Run("unchanged", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			ok, err := tf.InitFingerprintCurrent(fsys, unit, dataDir, recorded, noInitInputs)
			require.NoError(b, err)
			require.True(b, ok)
		}
	})

	b.Run("generated locals changed", func(b *testing.B) {
		regenerated := recorded
		regenerated.Files = "stale"

		b.ReportAllocs()

		for b.Loop() {
			ok, err := tf.InitFingerprintCurrent(fsys, unit, dataDir, regenerated, noInitInputs)
			require.NoError(b, err)
			require.True(b, ok)
		}
	})
}
