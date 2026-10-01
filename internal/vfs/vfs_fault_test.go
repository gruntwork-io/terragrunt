package vfs_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileExistsReportsStatFailure(t *testing.T) {
	t.Parallel()

	fsys := &faultFS{FS: vfs.NewMemMapFS(), faults: map[string]string{faultStat: ""}}

	exists, err := vfs.FileExists(fsys, "/anything")
	require.ErrorIs(t, err, errInjected)
	assert.False(t, exists)
}

func TestLstatWithoutLstaterFollowsLinks(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/target", []byte("content"), 0o644))
	require.NoError(t, vfs.Symlink(base, "/target", "/link"))

	info, err := vfs.Lstat(&faultFS{FS: base}, "/link")
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "a filesystem without Lstat describes what the link points at")
	assert.Equal(t, int64(len("content")), info.Size())
}

func TestWriteFileReportsMkdirFailure(t *testing.T) {
	t.Parallel()

	fsys := afero.NewReadOnlyFs(vfs.NewMemMapFS())

	require.ErrorIs(t, vfs.WriteFile(fsys, "/dir/file", []byte("x"), 0o644), syscall.EPERM)
}

func TestReadFileAsStringNamesTheFailingPath(t *testing.T) {
	t.Parallel()

	got, err := vfs.ReadFileAsString(vfs.NewMemMapFS(), "/missing/file")
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.Contains(t, err.Error(), "/missing/file")
	assert.Empty(t, got)
}

func TestReadFileLimitReportsCloseFailure(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/file", []byte("content"), 0o644))

	_, err := vfs.ReadFileLimit(&faultFS{FS: base, fileFaults: []string{faultClose}}, "/file", 3)
	require.ErrorIs(t, err, errInjected)
}

func TestParentPathHasSymlinkReportsStatFailure(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/root/a/b", []byte("x"), 0o644))

	fsys := &faultFS{FS: base, faults: map[string]string{faultStat: "/root/a"}}

	hasSymlink, err := vfs.ParentPathHasSymlink(fsys, "/root", "a/b")
	require.ErrorIs(t, err, errInjected)
	assert.False(t, hasSymlink)
}

func TestSymlinkWithoutLinkerSupport(t *testing.T) {
	t.Parallel()

	err := vfs.Symlink(&faultFS{FS: vfs.NewMemMapFS()}, "/target", "/link")

	var linkErr *os.LinkError
	require.ErrorAs(t, err, &linkErr)
	require.ErrorIs(t, err, afero.ErrNoSymlink)
	assert.Equal(t, "/target", linkErr.Old)
	assert.Equal(t, "/link", linkErr.New)
}

func TestStreamFileAtomicCleansUpAfterFailure(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		faults     map[string]string
		name       string
		fileFaults []string
	}{
		{
			name:   "scratch file cannot be created",
			faults: map[string]string{faultOpenFile: ""},
		},
		{
			name:   "scratch file mode cannot be set",
			faults: map[string]string{faultChmod: ""},
		},
		{
			name:       "scratch file cannot be closed",
			fileFaults: []string{faultClose},
		},
		{
			name:   "scratch file cannot be renamed into place",
			faults: map[string]string{faultRename: ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(base, "/dir/dest", []byte("previous"), 0o644))

			fsys := &faultFS{FS: base, faults: tc.faults, fileFaults: tc.fileFaults}

			err := vfs.WriteFileAtomic(fsys, "/dir/dest", []byte("replacement"), 0o644)
			require.ErrorIs(t, err, errInjected)

			got, err := vfs.ReadFile(base, "/dir/dest")
			require.NoError(t, err)
			assert.Equal(t, []byte("previous"), got, "a failed write must leave the previous file")

			entries, err := vfs.ReadDir(base, "/dir")
			require.NoError(t, err)
			require.Len(t, entries, 1, "the scratch file must not be left behind")
			assert.Equal(t, "dest", entries[0].Name())
		})
	}

	t.Run("parent directory cannot be created", func(t *testing.T) {
		t.Parallel()

		fsys := afero.NewReadOnlyFs(vfs.NewMemMapFS())

		err := vfs.WriteFileAtomic(fsys, "/dir/dest", []byte("x"), 0o644)
		require.ErrorIs(t, err, syscall.EPERM)
	})
}

func TestMirrorToMemReportsUnreadableFile(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/src/a.hcl", []byte("a"), 0o644))
	require.NoError(t, vfs.WriteFile(base, "/src/b.hcl", []byte("b"), 0o644))

	src := &faultFS{FS: base, faults: map[string]string{faultOpen: "/src/b.hcl"}}

	dst, err := vfs.MirrorToMem(src, "/src", vfs.MirrorLimits{MaxFiles: 10, MaxBytes: 1 << 20})
	require.ErrorIs(t, err, errInjected)
	assert.Nil(t, dst)
}

func TestMirrorToMemReportsUndescribableEntry(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(base, "/src/a.hcl", []byte("a"), 0o644))

	dst, err := vfs.MirrorToMem(infoFailFS{FS: base}, "/src", vfs.MirrorLimits{MaxFiles: 10, MaxBytes: 1 << 20})
	require.ErrorIs(t, err, errInjected)
	assert.Nil(t, dst)
}

func TestReadDirWithoutReadDirFile(t *testing.T) {
	t.Parallel()

	base := vfs.NewMemMapFS()
	for _, name := range []string{"c", "a", "b"} {
		require.NoError(t, vfs.WriteFile(base, filepath.Join("/dir", name), []byte(name), 0o644))
	}

	// faultFS hands out files that only offer Readdir, so ReadDir takes its
	// fallback path.
	fsys := &faultFS{FS: base}

	t.Run("entries come back sorted by name", func(t *testing.T) {
		t.Parallel()

		entries, err := vfs.ReadDir(fsys, "/dir")
		require.NoError(t, err)

		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}

		assert.Equal(t, []string{"a", "b", "c"}, names)
	})

	t.Run("a file is not a directory", func(t *testing.T) {
		t.Parallel()

		entries, err := vfs.ReadDir(fsys, "/dir/a")
		require.Error(t, err)
		assert.Nil(t, entries)
	})
}

func TestReadDirReportsReadDirFailure(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/file", []byte("x"), 0o644))

	entries, err := vfs.ReadDir(fsys, "/file")
	require.Error(t, err)
	assert.Nil(t, entries)
}

// errInjected is the failure faultFS and faultFile report.
var errInjected = errors.New("injected failure")

// Operations faultFS and faultFile can be told to fail.
const (
	faultOpen     = "open"
	faultOpenFile = "openfile"
	faultStat     = "stat"
	faultMkdirAll = "mkdirall"
	faultChmod    = "chmod"
	faultRename   = "rename"
	faultRemove   = "remove"
	faultClose    = "close"
	faultRead     = "read"
)

// faultFS wraps a filesystem and fails the operations named in faults, each
// for the one path it maps to, or for every path when that is empty. Every
// file it opens is a faultFile failing the operations in fileFaults.
//
// It embeds only vfs.FS, so the optional interfaces of the wrapped filesystem
// (Lstat, symlinks, hard links, clones, locks) are hidden and callers take
// their fallback paths.
type faultFS struct {
	vfs.FS
	faults     map[string]string
	fileFaults []string
}

func (fsys *faultFS) fail(op, name string) error {
	path, ok := fsys.faults[op]
	if !ok || (path != "" && filepath.Clean(path) != filepath.Clean(name)) {
		return nil
	}

	return &os.PathError{Op: op, Path: name, Err: errInjected}
}

func (fsys *faultFS) wrap(file vfs.File, err error) (vfs.File, error) {
	if err != nil {
		return nil, err
	}

	return &faultFile{File: file, faults: fsys.fileFaults}, nil
}

func (fsys *faultFS) Open(name string) (vfs.File, error) {
	if err := fsys.fail(faultOpen, name); err != nil {
		return nil, err
	}

	return fsys.wrap(fsys.FS.Open(name))
}

func (fsys *faultFS) OpenFile(name string, flag int, perm os.FileMode) (vfs.File, error) {
	if err := fsys.fail(faultOpenFile, name); err != nil {
		return nil, err
	}

	return fsys.wrap(fsys.FS.OpenFile(name, flag, perm))
}

func (fsys *faultFS) Stat(name string) (os.FileInfo, error) {
	if err := fsys.fail(faultStat, name); err != nil {
		return nil, err
	}

	return fsys.FS.Stat(name)
}

func (fsys *faultFS) MkdirAll(path string, perm os.FileMode) error {
	if err := fsys.fail(faultMkdirAll, path); err != nil {
		return err
	}

	return fsys.FS.MkdirAll(path, perm)
}

func (fsys *faultFS) Chmod(name string, mode os.FileMode) error {
	if err := fsys.fail(faultChmod, name); err != nil {
		return err
	}

	return fsys.FS.Chmod(name, mode)
}

func (fsys *faultFS) Rename(oldname, newname string) error {
	if err := fsys.fail(faultRename, oldname); err != nil {
		return err
	}

	return fsys.FS.Rename(oldname, newname)
}

func (fsys *faultFS) Remove(name string) error {
	if err := fsys.fail(faultRemove, name); err != nil {
		return err
	}

	return fsys.FS.Remove(name)
}

// faultFile wraps a file and fails the operations named in faults. A close
// still closes the wrapped file before failing. It offers Readdir but not
// ReadDir, as a file from an older filesystem backing would.
type faultFile struct {
	vfs.File
	faults []string
}

func (f *faultFile) failing(op string) bool {
	return slices.Contains(f.faults, op)
}

func (f *faultFile) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}

	if f.failing(faultClose) {
		return errInjected
	}

	return nil
}

func (f *faultFile) Read(p []byte) (int, error) {
	if f.failing(faultRead) {
		return 0, errInjected
	}

	return f.File.Read(p)
}

func (f *faultFile) Stat() (os.FileInfo, error) {
	if f.failing(faultStat) {
		return nil, errInjected
	}

	return f.File.Stat()
}

// infoFailFS hands out directories whose entries cannot describe themselves,
// as entries removed between the listing and the stat would.
type infoFailFS struct {
	vfs.FS
}

func (fsys infoFailFS) Open(name string) (vfs.File, error) {
	file, err := fsys.FS.Open(name)
	if err != nil {
		return nil, err
	}

	return infoFailDir{File: file}, nil
}

type infoFailDir struct {
	vfs.File
}

func (d infoFailDir) ReadDir(n int) ([]fs.DirEntry, error) {
	infos, err := d.Readdir(n)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	entries := make([]fs.DirEntry, 0, len(infos))
	for _, info := range infos {
		entries = append(entries, infoFailEntry{DirEntry: fs.FileInfoToDirEntry(info)})
	}

	return entries, nil
}

type infoFailEntry struct {
	fs.DirEntry
}

func (infoFailEntry) Info() (fs.FileInfo, error) {
	return nil, errInjected
}
