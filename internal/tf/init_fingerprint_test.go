package tf_test

import (
	"path/filepath"
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
)

// writeInitializedUnit lays out under base an initialized unit at base/unit that calls a module
// inside the unit, one outside it, and one init downloaded, and returns the unit directory.
func writeInitializedUnit(t *testing.T, fsys vfs.FS, base string) string {
	t.Helper()

	unit := filepath.Join(base, "unit")
	files := map[string]string{
		filepath.Join(unit, "main.tf"):                                          fingerprintRootTF,
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
			name: "root module file edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), fingerprintRootTF+"\n# edited\n")
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
			name: "module inside the unit edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()
				writeTestFile(t, fsys, filepath.Join(unit, "example", "main.tf"), fingerprintChildTF+"\n# edited\n")
			},
			wantChanged: true,
		},
		{
			name: "module outside the unit edited",
			mutate: func(t *testing.T, fsys vfs.FS, unit string) {
				t.Helper()

				outside := filepath.Join(unit, "..", "modules", "outside", "main.tf")
				writeTestFile(t, fsys, outside, fingerprintChildTF+"\n# edited\n")
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
			dataDir := filepath.Join(unit, ".terraform")

			before, err := tf.InitFingerprint(fsys, unit, dataDir)
			require.NoError(t, err)

			tc.mutate(t, fsys, unit)

			after, err := tf.InitFingerprint(fsys, unit, dataDir)
			require.NoError(t, err)

			assert.Equal(t, tc.wantChanged, before != after)
		})
	}
}

func TestInitFingerprintIgnoresWhereTheUnitLives(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	here := writeInitializedUnit(t, fsys, "/here")
	there := writeInitializedUnit(t, fsys, "/there/deeper")

	hereFingerprint, err := tf.InitFingerprint(fsys, here, filepath.Join(here, ".terraform"))
	require.NoError(t, err)

	thereFingerprint, err := tf.InitFingerprint(fsys, there, filepath.Join(there, ".terraform"))
	require.NoError(t, err)

	assert.Equal(t, hereFingerprint, thereFingerprint)
}

func TestInitFingerprintBeforeAnyInit(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	unit := "/work/unit"
	writeTestFile(t, fsys, filepath.Join(unit, "main.tf"), fingerprintChildTF)

	fingerprint, err := tf.InitFingerprint(fsys, unit, filepath.Join(unit, ".terraform"))
	require.NoError(t, err)
	assert.NotEmpty(t, fingerprint)
}

func TestInitFingerprintErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing root module", func(t *testing.T) {
		t.Parallel()

		_, err := tf.InitFingerprint(vfs.NewMemMapFS(), "/missing", "/missing/.terraform")
		require.Error(t, err)
	})

	t.Run("unparseable modules manifest", func(t *testing.T) {
		t.Parallel()

		fsys := vfs.NewMemMapFS()
		unit := writeInitializedUnit(t, fsys, "/work")
		writeTestFile(t, fsys, filepath.Join(unit, ".terraform", "modules", "modules.json"), "{")

		_, err := tf.InitFingerprint(fsys, unit, filepath.Join(unit, ".terraform"))
		require.Error(t, err)
	})
}

func TestWriteInitFingerprintCreatesTheDataDir(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	dataDir := "/work/unit/.terraform"

	require.NoError(t, tf.WriteInitFingerprint(fsys, dataDir, "abc"))

	content, err := vfs.ReadFile(fsys, filepath.Join(dataDir, tf.InitFingerprintFile))
	require.NoError(t, err)
	assert.Equal(t, "abc", string(content))
}

func writeTestFile(t *testing.T, fsys vfs.FS, path, content string) {
	t.Helper()

	require.NoError(t, fsys.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
}
