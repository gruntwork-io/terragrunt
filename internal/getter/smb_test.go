package getter_test

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmbClientGetterDownloadsDirectory(t *testing.T) {
	t.Parallel()

	log := &commandLog{replies: map[string]vexec.Result{
		"smbclient -U user //host/share --directory . -c allinfo modules/vpc": {
			Stdout: []byte("attributes: D (10)\n"),
		},
	}}
	v := gitVenv(log)
	dst := venvtest.Root("/work/module")

	require.NoError(t, download(t, getter.NewSmbClientGetter(v),
		"smb://user:pw@host/share/modules/vpc", dst, getter.ModeDir))

	assert.Equal(t, []string{
		"smbclient -U user //host/share --directory . -c allinfo modules/vpc",
		"smbclient -U user //host/share --directory modules/vpc -c prompt OFF;recurse ON; mget *",
	}, log.argv())
	assert.Equal(t, "pw", log.envOf(t, "smbclient -U user //host/share --directory modules/vpc", "PASSWD"))
	assert.NotContains(t, log.trusted, false, "every smbclient command must run as Terragrunt's own")

	info, err := v.FS.Stat(dst)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestSmbClientGetterPassesPortSeparately(t *testing.T) {
	t.Parallel()

	log := &commandLog{replies: map[string]vexec.Result{"smbclient": {Stdout: []byte("attributes: D (10)\n")}}}

	_, err := getter.NewSmbClientGetter(gitVenv(log)).
		Mode(t.Context(), &url.URL{Scheme: "smb", Host: "host:1445", Path: "/share/dir/main.tf"})
	require.NoError(t, err)

	assert.Equal(t, []string{"smbclient -N -p 1445 //host/share --directory dir -c allinfo main.tf"}, log.argv())
}

func TestSmbClientGetterReportsMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		wantE error
		name  string
		info  string
		want  getter.Mode
	}{
		{name: "directory", info: "attributes: D (10)\n", want: getter.ModeDir},
		{name: "file", info: "attributes: A (20)\n", want: getter.ModeFile},
		{name: "missing", info: "NT_STATUS_OBJECT_NAME_NOT_FOUND\n", wantE: getter.ErrSMBNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			log := &commandLog{replies: map[string]vexec.Result{"smbclient": {Stdout: []byte(tc.info)}}}

			mode, err := getter.NewSmbClientGetter(gitVenv(log)).
				Mode(t.Context(), &url.URL{Scheme: "smb", Host: "host", Path: "/share/dir/main.tf"})

			if tc.wantE != nil {
				require.ErrorIs(t, err, tc.wantE)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, mode)
			assert.Equal(t, []string{"smbclient -N //host/share --directory dir -c allinfo main.tf"}, log.argv())
		})
	}
}

func TestSmbClientGetterRefusesCommandSeparators(t *testing.T) {
	t.Parallel()

	log := &commandLog{}

	_, err := getter.NewSmbClientGetter(gitVenv(log)).
		Mode(t.Context(), &url.URL{Scheme: "smb", Host: "host", Path: "/share/x;!touch pwned"})
	require.ErrorIs(t, err, getter.ErrUnsafeSMBPath)
	assert.Empty(t, log.argv())
}

func TestSmbClientGetterKeepsPasswordOutOfErrors(t *testing.T) {
	t.Parallel()

	const password = "fake-secret-smb"

	log := &commandLog{replies: map[string]vexec.Result{"smbclient": {Err: errDenied}}}

	err := download(t, getter.NewSmbClientGetter(gitVenv(log)),
		"smb://user:"+password+"@host/share/modules/vpc", venvtest.Root("/work/module"), getter.ModeDir)
	require.ErrorIs(t, err, errDenied)
	assert.NotContains(t, err.Error(), password)
}

func TestDownloadKeepsSMBPasswordOutOfErrorWhenEverySMBGetterFails(t *testing.T) {
	t.Parallel()

	const password = "fake-secret-smb"

	log := &commandLog{replies: map[string]vexec.Result{"smbclient": {Err: errDenied}}}
	v := gitVenv(log).WithGOOS("linux")

	_, err := getter.Download(t.Context(), getter.NewClient(logger.CreateLogger(), v), &getter.Request{
		Src:     "smb://user:" + password + "@host/share/modules/vpc",
		Dst:     venvtest.Root("/work/module"),
		GetMode: getter.ModeDir,
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), password)
	assert.Len(t, log.argv(), 1, "the smbclient getter must have run before the error")
}

func TestSmbMountGetterCopiesMountedShare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		goos  string
		mount string
	}{
		{name: "windows", goos: "windows", mount: `\\` + filepath.Join("host", "/share/modules")},
		{name: "darwin", goos: "darwin", mount: "/" + filepath.Join("Volumes", "/share/modules")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if (tc.goos == "windows") != helpers.IsWindows() {
				t.Skipf("Skipping: a %s mount path is absolute only on a %s host", tc.goos, tc.goos)
			}

			log := &commandLog{}
			v := gitVenv(log).WithGOOS(tc.goos)
			dst := venvtest.Root("/work/module")

			require.NoError(t, v.FS.MkdirAll(tc.mount, 0o755))
			require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(tc.mount, "main.tf"), []byte("# vpc"), 0o644))

			g := getter.NewSmbMountGetter(v, getter.NewFileCopyGetter(v.FS).WithLogger(logger.CreateLogger()))
			require.NoError(t, download(t, g, "smb://host/share/modules", dst, getter.ModeDir))

			info, err := vfs.Lstat(v.FS, dst)
			require.NoError(t, err)
			assert.True(t, info.IsDir(), "the destination must be a copy, not a link")

			assertFileContent(t, v.FS, filepath.Join(dst, "main.tf"), "# vpc")
			assert.Empty(t, log.argv())
		})
	}
}

func TestSmbMountGetterCopiesMountedFile(t *testing.T) {
	t.Parallel()

	v := gitVenv(&commandLog{}).WithGOOS("windows")
	mount := `\\` + filepath.Join("host", "/share/modules")
	dst := venvtest.Root("/work/main.tf")

	require.NoError(t, v.FS.MkdirAll(mount, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(mount, "main.tf"), []byte("# vpc"), 0o644))

	g := getter.NewSmbMountGetter(v, getter.NewFileCopyGetter(v.FS).WithLogger(logger.CreateLogger()))
	require.NoError(t, download(t, g, "smb://host/share/modules/main.tf", dst, getter.ModeFile))

	assertFileContent(t, v.FS, dst, "# vpc")
}

func TestSmbMountGetterDetectsByVenvPlatform(t *testing.T) {
	t.Parallel()

	tests := []struct {
		goos string
		want bool
	}{
		{goos: "linux", want: false},
		{goos: "darwin", want: true},
		{goos: "windows", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.goos, func(t *testing.T) {
			t.Parallel()

			v := gitVenv(&commandLog{}).WithGOOS(tc.goos)
			g := getter.NewSmbMountGetter(v, getter.NewFileCopyGetter(v.FS))

			got, err := g.Detect(&getter.Request{Src: "smb://host/share/modules"})
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
