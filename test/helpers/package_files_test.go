package helpers_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupContextOutlivesTestContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := helpers.CleanupContext(t)
	defer cancel()

	deadline, ok := ctx.Deadline()
	require.True(t, ok, "cleanup context is bounded by a timeout")
	assert.WithinDuration(t, time.Now().Add(2*time.Minute), deadline, 5*time.Second)
	require.NoError(t, ctx.Err())

	cancel()
	require.Error(t, ctx.Err(), "the returned cancel function ends the context")
}

func TestCopyEnvironment(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	pkgTWriteFiles(t, src, map[string]string{
		"main.tf":                                             "# main",
		filepath.Join("sub", "unit.hcl"):                      "# unit",
		filepath.Join(".hidden", "keep"):                      "hidden",
		filepath.Join(".terraform", "providers"):              "cache",
		filepath.Join("sub", ".terragrunt-cache", "leftover"): "cache",
		filepath.Join("sub", "terragrunt-debug.tfvars.json"):  "{}",
	})

	testCases := []struct {
		name          string
		include       []string
		wantHiddenDir bool
	}{
		{
			name:          "hidden entries are skipped by default",
			wantHiddenDir: false,
		},
		{
			name:          "include pattern pulls a hidden entry back in",
			include:       []string{".hidden/**"},
			wantHiddenDir: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.CopyEnvironment(t, src, tc.include...)

			// The fixture lands beneath the returned directory at its own
			// (volume-stripped) absolute path.
			dst := filepath.Join(tmpDir, pkgTStripVolume(src))

			assert.FileExists(t, filepath.Join(dst, "main.tf"))
			assert.FileExists(t, filepath.Join(dst, "sub", "unit.hcl"))
			assert.NoDirExists(t, filepath.Join(dst, ".terraform"))
			assert.NoDirExists(t, filepath.Join(dst, "sub", ".terragrunt-cache"))
			assert.NoFileExists(t, filepath.Join(dst, "sub", "terragrunt-debug.tfvars.json"))

			if tc.wantHiddenDir {
				assert.FileExists(t, filepath.Join(dst, ".hidden", "keep"))
			} else {
				assert.NoDirExists(t, filepath.Join(dst, ".hidden"))
			}
		})
	}
}

func TestCreateTmpTerragruntConfig(t *testing.T) {
	t.Parallel()

	templates := t.TempDir()
	pkgTWriteFiles(t, templates, map[string]string{
		"terragrunt.hcl": pkgTPlaceholderTemplate,
	})

	got := helpers.CreateTmpTerragruntConfig(t, templates, "my-bucket", "my-table", "terragrunt.hcl")

	assert.Equal(t, "terragrunt.hcl", filepath.Base(got))
	assert.NotEqual(t, templates, filepath.Dir(got), "the config is written to a fresh directory")
	assert.Equal(
		t,
		"bucket=my-bucket\ntable=my-table\nregion=not-used\nlogs=my-bucket-tf-state-logs\n",
		pkgTReadFile(t, got),
	)
}

func TestCreateTmpTerragruntConfigContent(t *testing.T) {
	t.Parallel()

	const contents = "inputs = {}\n"

	got := helpers.CreateTmpTerragruntConfigContent(t, contents, "root.hcl")

	assert.Equal(t, "root.hcl", filepath.Base(got))
	assert.Equal(t, contents, pkgTReadFile(t, got))
}

func TestCopyTerragruntConfigAndFillPlaceholders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.hcl")
	dst := filepath.Join(dir, "dst.hcl")
	pkgTWriteFiles(t, dir, map[string]string{"src.hcl": pkgTPlaceholderTemplate})

	helpers.CopyTerragruntConfigAndFillPlaceholders(t, src, dst, "bkt", "tbl", "eu-west-1")

	assert.Equal(
		t,
		"bucket=bkt\ntable=tbl\nregion=eu-west-1\nlogs=bkt-tf-state-logs\n",
		pkgTReadFile(t, dst),
	)
	assert.Equal(t, pkgTPlaceholderTemplate, pkgTReadFile(t, src), "the source is left untouched")
}

func TestCopyAndFillMapPlaceholders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "dst.txt")
	pkgTWriteFiles(t, dir, map[string]string{"src.txt": "__A__ and __A__, __B__, __C__"})

	helpers.CopyAndFillMapPlaceholders(t, src, dst, map[string]string{
		"__A__": "alpha",
		"__B__": "beta",
	})

	assert.Equal(t, "alpha and alpha, beta, __C__", pkgTReadFile(t, dst))
}

func TestCreateTmpTerragruntConfigWithParentAndChild(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	childRel := filepath.Join("child", "unit")
	pkgTWriteFiles(t, parent, map[string]string{
		"root.hcl": "parent " + pkgTPlaceholderTemplate,
		filepath.Join(childRel, "terragrunt.hcl"): "child " + pkgTPlaceholderTemplate,
	})

	got := helpers.CreateTmpTerragruntConfigWithParentAndChild(
		t,
		parent,
		childRel,
		"bkt",
		"root.hcl",
		"terragrunt.hcl",
	)

	require.True(t, filepath.IsAbs(got))
	assert.Equal(t, filepath.Join(childRel, "terragrunt.hcl"), pkgTTail(got, 3))

	const filled = "bucket=bkt\ntable=not-used\nregion=not-used\nlogs=bkt-tf-state-logs\n"

	assert.Equal(t, "child "+filled, pkgTReadFile(t, got))

	root := filepath.Dir(filepath.Dir(filepath.Dir(got)))
	assert.Equal(t, "parent "+filled, pkgTReadFile(t, filepath.Join(root, "root.hcl")))
}

func TestUniqueID(t *testing.T) {
	t.Parallel()

	base62 := regexp.MustCompile(`^[0-9A-Za-z]{6}$`)
	seen := make(map[string]struct{})

	for range 50 {
		id := helpers.UniqueID()
		assert.Regexp(t, base62, id)

		seen[id] = struct{}{}
	}

	assert.Greater(t, len(seen), 1, "ids are random, not constant")
}

func TestFileIsInFolder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		filepath.Join("a", "b", "needle.txt"): "x",
		"top.txt":                             "x",
	})

	assert.True(t, helpers.FileIsInFolder(t, "needle.txt", dir))
	assert.True(t, helpers.FileIsInFolder(t, "top.txt", dir))
	assert.True(t, helpers.FileIsInFolder(t, "b", dir), "directories match by name too")
	assert.False(t, helpers.FileIsInFolder(t, "missing.txt", dir))
}

func TestFindFilesWithExtension(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		"a.tf":                          "",
		filepath.Join("nested", "b.tf"): "",
		"c.hcl":                         "",
	})
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dir.tf"), 0o755))

	files, err := helpers.FindFilesWithExtension(dir, ".tf")
	require.NoError(t, err)

	slices.Sort(files)
	assert.Equal(t, []string{filepath.Join(dir, "a.tf"), filepath.Join(dir, "nested", "b.tf")}, files)

	_, err = helpers.FindFilesWithExtension(filepath.Join(dir, "missing"), ".tf")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCleanupTerraformFolder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		helpers.TerraformState:                              "{}",
		helpers.TerraformStateBackup:                        "{}",
		helpers.TerragruntDebugFile:                         "{}",
		filepath.Join(helpers.TerraformFolder, "providers"): "x",
		"main.tf": "# keep",
	})

	helpers.CleanupTerraformFolder(t, dir)

	assert.NoFileExists(t, filepath.Join(dir, helpers.TerraformState))
	assert.NoFileExists(t, filepath.Join(dir, helpers.TerraformStateBackup))
	assert.NoFileExists(t, filepath.Join(dir, helpers.TerragruntDebugFile))
	assert.NoDirExists(t, filepath.Join(dir, helpers.TerraformFolder))
	assert.FileExists(t, filepath.Join(dir, "main.tf"))

	// Running it again over the already-clean folder is a no-op.
	helpers.CleanupTerraformFolder(t, dir)
}

func TestCleanupTerragruntFolder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		filepath.Join(helpers.TerragruntCache, "abc", "main.tf"): "x",
		"terragrunt.hcl": "# keep",
	})

	helpers.CleanupTerragruntFolder(t, dir)

	assert.NoDirExists(t, filepath.Join(dir, helpers.TerragruntCache))
	assert.FileExists(t, filepath.Join(dir, "terragrunt.hcl"))
}

func TestRemoveFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{"f.txt": "x"})

	helpers.RemoveFile(t, filepath.Join(dir, "f.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "f.txt"))

	// A missing file is not an error.
	helpers.RemoveFile(t, filepath.Join(dir, "f.txt"))
}

func TestRemoveFolder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{filepath.Join("d", "e", "f.txt"): "x"})

	helpers.RemoveFolder(t, filepath.Join(dir, "d"))
	assert.NoDirExists(t, filepath.Join(dir, "d"))

	// A missing folder is not an error.
	helpers.RemoveFolder(t, filepath.Join(dir, "d"))
}

func TestCreateEmptyStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	helpers.CreateEmptyStateFile(t, dir)

	info, err := os.Stat(filepath.Join(dir, helpers.TerraformState))
	require.NoError(t, err)
	assert.Zero(t, info.Size())
}

func TestHCLFilesInDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pkgTWriteFiles(t, dir, map[string]string{
		"root.hcl":                           "",
		filepath.Join("a", "terragrunt.hcl"): "",
		filepath.Join("a", "main.tf"):        "",
	})
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dir.hcl"), 0o755))

	files := helpers.HCLFilesInDir(t, dir)

	slices.Sort(files)
	assert.Equal(t, []string{filepath.Join(dir, "a", "terragrunt.hcl"), filepath.Join(dir, "root.hcl")}, files)
}

func TestCopyDir(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	pkgTWriteFiles(t, src, map[string]string{
		"a.txt":                              "a",
		filepath.Join("nested", "deep", "b"): "b",
	})
	require.NoError(t, os.Mkdir(filepath.Join(src, "empty"), 0o755))

	dst := filepath.Join(t.TempDir(), "copy")

	helpers.CopyDir(t, src, dst)

	assert.Equal(t, "a", pkgTReadFile(t, filepath.Join(dst, "a.txt")))
	assert.Equal(t, "b", pkgTReadFile(t, filepath.Join(dst, "nested", "deep", "b")))
	assert.DirExists(t, filepath.Join(dst, "empty"))
}

func TestCopyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src.sh")
	dst := filepath.Join(dir, "dst.sh")

	require.NoError(t, os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o750))

	helpers.CopyFile(t, src, dst)

	assert.Equal(t, "#!/bin/sh\necho hi\n", pkgTReadFile(t, dst))

	srcInfo, err := os.Stat(src)
	require.NoError(t, err)

	dstInfo, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, srcInfo.Mode(), dstInfo.Mode(), "permissions are preserved")
}

func TestValidateOutput(t *testing.T) {
	t.Parallel()

	outputs := map[string]helpers.TerraformOutput{
		"name":  {Value: "app"},
		"count": {Value: float64(3)},
	}

	helpers.ValidateOutput(t, outputs, "name", "app")
	helpers.ValidateOutput(t, outputs, "count", float64(3))
}

func TestRunVenvUsesPerTestConfigDir(t *testing.T) {
	t.Parallel()

	first := helpers.RunVenv(t)
	second := helpers.RunVenv(t)

	require.NotNil(t, first.Platform)

	firstDir, err := first.Platform.UserConfigDir()
	require.NoError(t, err)
	assert.DirExists(t, firstDir)

	secondDir, err := second.Platform.UserConfigDir()
	require.NoError(t, err)
	assert.NotEqual(t, firstDir, secondDir, "each call gets its own config dir")

	assert.NotNil(t, first.Env, "the rest of the OS venv is kept")
}

func TestIsTerragruntProviderCacheEnabled(t *testing.T) {
	testCases := []struct {
		legacy string
		name   string
		value  string
		want   bool
	}{
		{
			name: "both unset",
			want: false,
		},
		{
			name:  "TG_PROVIDER_CACHE enabled",
			value: "true",
			want:  true,
		},
		{
			name:   "legacy variable enabled",
			legacy: "1",
			want:   true,
		},
		{
			name:   "legacy disabled does not hide TG_PROVIDER_CACHE",
			legacy: "false",
			value:  "true",
			want:   true,
		},
		{
			name:   "both disabled",
			legacy: "false",
			value:  "0",
			want:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TERRAGRUNT_PROVIDER_CACHE", tc.legacy)
			t.Setenv("TG_PROVIDER_CACHE", tc.value)

			assert.Equal(t, tc.want, helpers.IsTerragruntProviderCacheEnabled(t))
		})
	}
}

func TestFakeProviderCreateMirror(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	provider := &helpers.FakeProvider{
		RegistryName: "registry.example.com",
		Namespace:    "acme",
		Name:         "null",
		Version:      "1.0.0",
		PlatformOS:   "linux",
		PlatformArch: "amd64",
	}
	provider.CreateMirror(t, root)

	// A second platform and version merge into the files the first call wrote.
	other := *provider
	other.Version = "2.0.0"
	other.PlatformOS = "darwin"
	other.PlatformArch = "arm64"
	other.CreateMirror(t, root)

	providerDir := filepath.Join(root, "registry.example.com", "acme", "null")

	var index struct {
		Versions map[string]any `json:"versions"`
	}

	pkgTReadJSON(t, filepath.Join(providerDir, "index.json"), &index)
	assert.Equal(t, map[string]any{"1.0.0": map[string]any{}, "2.0.0": map[string]any{}}, index.Versions)

	var version struct {
		Archives map[string]struct {
			URL string `json:"url"`
		} `json:"archives"`
	}

	pkgTReadJSON(t, filepath.Join(providerDir, "1.0.0.json"), &version)
	require.Contains(t, version.Archives, "linux_amd64")
	assert.Equal(t, "terraform-provider-null_1.0.0_linux_amd64.zip", version.Archives["linux_amd64"].URL)

	assert.FileExists(t, filepath.Join(providerDir, "terraform-provider-null_1.0.0_linux_amd64.zip"))
	assert.FileExists(t, filepath.Join(providerDir, "terraform-provider-null_2.0.0_darwin_arm64.zip"))
	assert.NoFileExists(
		t,
		filepath.Join(providerDir, "terraform-provider-null_v1.0.0_x5"),
		"the raw binary is removed once archived",
	)
}

const pkgTPlaceholderTemplate = "bucket=__FILL_IN_BUCKET_NAME__\n" +
	"table=__FILL_IN_LOCK_TABLE_NAME__\n" +
	"region=__FILL_IN_REGION__\n" +
	"logs=__FILL_IN_LOGS_BUCKET_NAME__\n"

// pkgTWriteFiles writes each relative path in files under root with its contents.
func pkgTWriteFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()

	for rel, contents := range files {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
	}
}

// pkgTReadFile returns the contents of path.
func pkgTReadFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// pkgTReadJSON decodes the JSON file at path into dest.
func pkgTReadJSON(t *testing.T, path string, dest any) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, dest))
}

// pkgTStripVolume drops the volume name (a Windows drive letter) from path.
func pkgTStripVolume(path string) string {
	return path[len(filepath.VolumeName(path)):]
}

// pkgTTail returns the last n elements of path joined back together.
func pkgTTail(path string, n int) string {
	parts := make([]string, 0, n)

	for range n {
		parts = append([]string{filepath.Base(path)}, parts...)
		path = filepath.Dir(path)
	}

	return filepath.Join(parts...)
}
