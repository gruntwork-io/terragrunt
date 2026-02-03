// Package vfs provides a virtual filesystem abstraction for testing and production use.
// It wraps afero to provide a consistent interface for filesystem operations.
package vfs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

// FS is the filesystem interface used throughout the codebase.
// It provides an abstraction over real and in-memory filesystems.
type FS = afero.Fs

// NewOSFS returns a filesystem backed by the real operating system filesystem.
func NewOSFS() FS {
	return afero.NewOsFs()
}

// NewMemMapFS returns an in-memory filesystem for testing purposes.
func NewMemMapFS() FS {
	return afero.NewMemMapFs()
}

// FileExists checks if a path exists using the given filesystem.
// Returns (true, nil) if the file exists, (false, nil) if it does not exist,
// and (false, error) for other errors (e.g., permission denied).
func FileExists(fs FS, path string) (bool, error) {
	_, err := fs.Stat(path)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

// WriteFile writes data to a file on the given filesystem.
func WriteFile(fs FS, filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(fs, filename, data, perm)
}

// ReadFile reads the contents of a file from the given filesystem.
func ReadFile(fs FS, filename string) ([]byte, error) {
	return afero.ReadFile(fs, filename)
}

// Symlink creates a symbolic link. It uses afero's SymlinkIfPossible
// which is supported by both OsFs and MemMapFs.
func Symlink(fs FS, oldname, newname string) error {
	linker, ok := fs.(afero.Linker)
	if !ok {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
	}

	return linker.SymlinkIfPossible(oldname, newname)
}

// ZipDecompressor handles zip archive extraction with configurable limits.
type ZipDecompressor struct {
	// FileSizeLimit limits total decompressed size in bytes. Zero means no limit.
	FileSizeLimit int64
	// FilesLimit limits the number of files. Zero means no limit.
	FilesLimit int
}

// ZipDecompressorOption is a functional option for configuring ZipDecompressor.
type ZipDecompressorOption func(*ZipDecompressor)

// WithFileSizeLimit sets the maximum total decompressed size in bytes.
// Zero means no limit.
func WithFileSizeLimit(limit int64) ZipDecompressorOption {
	return func(z *ZipDecompressor) {
		z.FileSizeLimit = limit
	}
}

// WithFilesLimit sets the maximum number of files that can be extracted.
// Zero means no limit.
func WithFilesLimit(limit int) ZipDecompressorOption {
	return func(z *ZipDecompressor) {
		z.FilesLimit = limit
	}
}

// NewZipDecompressor creates a new ZipDecompressor with the given options.
func NewZipDecompressor(opts ...ZipDecompressorOption) *ZipDecompressor {
	z := &ZipDecompressor{}
	for _, opt := range opts {
		opt(z)
	}

	return z
}

// Unzip extracts a zip archive from src to dst directory on the given filesystem.
// The umask parameter is applied to file permissions (use 0 to preserve original permissions).
func (z *ZipDecompressor) Unzip(l log.Logger, fs FS, dst, src string, umask os.FileMode) error {
	file, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open zip archive %q: %w", src, err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			l.Warnf("Error closing zip archive %q: %v", src, closeErr)
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat zip archive %q: %w", src, err)
	}

	size := fileInfo.Size()

	var readerAt io.ReaderAt
	if ra, ok := file.(io.ReaderAt); ok {
		readerAt = ra
	} else {
		data, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("failed to read zip archive %q: %w", src, err)
		}

		readerAt = bytes.NewReader(data)
		size = int64(len(data))
	}

	zipReader, err := zip.NewReader(readerAt, size)
	if err != nil {
		return fmt.Errorf("failed to read zip archive %q: %w", src, err)
	}

	if err := fs.MkdirAll(dst, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dst, err)
	}

	if z.FilesLimit > 0 && len(zipReader.File) > z.FilesLimit {
		return fmt.Errorf(
			"zip archive contains %d files, exceeds limit of %d",
			len(zipReader.File),
			z.FilesLimit,
		)
	}

	var totalSize int64

	for _, zipFile := range zipReader.File {
		if err := z.extractZipFile(l, fs, dst, zipFile, umask, &totalSize); err != nil {
			return fmt.Errorf("failed to extract file %q: %w", zipFile.Name, err)
		}
	}

	return nil
}

// containsDotDot checks if a path contains ".." as a path component.
// This is more precise than strings.Contains(name, "..") which would
// reject legitimate files like "file..txt".
func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}

	return slices.Contains(strings.FieldsFunc(v, func(r rune) bool {
		return r == '/' || r == '\\'
	}), "..")
}

// sanitizeZipPath validates and sanitizes a zip entry path to prevent ZipSlip attacks.
func sanitizeZipPath(dst, name string) (string, error) {
	if containsDotDot(name) {
		return "", fmt.Errorf("illegal file path in zip: %s", name)
	}

	destPath := filepath.Join(dst, filepath.Clean(name))

	if !strings.HasPrefix(destPath, filepath.Clean(dst)+string(os.PathSeparator)) {
		return "", fmt.Errorf("illegal destination path in zip: %s", destPath)
	}

	return destPath, nil
}

// extractZipFile extracts a single file from a zip archive.
func (z *ZipDecompressor) extractZipFile(l log.Logger, fs FS, dst string, zipFile *zip.File, umask os.FileMode, totalSize *int64) error {
	destPath, err := sanitizeZipPath(dst, zipFile.Name)
	if err != nil {
		return err
	}

	fileInfo := zipFile.FileInfo()

	if fileInfo.IsDir() {
		if err := fs.MkdirAll(destPath, applyUmask(fileInfo.Mode(), umask)); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", destPath, err)
		}

		return nil
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return extractSymlink(l, fs, dst, destPath, zipFile)
	}

	return z.extractRegularFile(l, fs, destPath, zipFile, umask, totalSize)
}

// validateSymlinkTarget validates that a symlink target doesn't escape the destination directory.
func validateSymlinkTarget(dst, linkPath, target string) error {
	// Resolve the target relative to the link's directory
	absTarget := target
	if !filepath.IsAbs(target) {
		absTarget = filepath.Join(filepath.Dir(linkPath), target)
	}

	absTarget = filepath.Clean(absTarget)
	cleanDst := filepath.Clean(dst)

	// Ensure it stays within dst
	if !strings.HasPrefix(absTarget, cleanDst+string(os.PathSeparator)) && absTarget != cleanDst {
		return fmt.Errorf("symlink target escapes destination: %s -> %s", linkPath, target)
	}

	return nil
}

// extractSymlink extracts a symlink from a zip file.
func extractSymlink(l log.Logger, fs FS, dst, destPath string, zipFile *zip.File) error {
	rc, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", zipFile.Name, err)
	}

	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", zipFile.Name, closeErr)
		}
	}()

	targetBytes, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", zipFile.Name, err)
	}

	target := string(targetBytes)

	// Validate symlink target doesn't escape destination
	if err := validateSymlinkTarget(dst, destPath, target); err != nil {
		return err
	}

	if err := fs.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	return Symlink(fs, target, destPath)
}

// extractRegularFile extracts a regular file from a zip file.
func (z *ZipDecompressor) extractRegularFile(
	l log.Logger,
	fs FS,
	destPath string,
	zipFile *zip.File,
	umask os.FileMode,
	totalSize *int64,
) error {
	if err := fs.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	rc, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", zipFile.Name, err)
	}

	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", zipFile.Name, closeErr)
		}
	}()

	mode := applyUmask(zipFile.FileInfo().Mode(), umask)

	outFile, err := fs.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", destPath, err)
	}

	var reader io.Reader = rc

	if z.FileSizeLimit > 0 {
		reader = &limitedReader{
			reader:    rc,
			remaining: z.FileSizeLimit - *totalSize,
			totalSize: totalSize,
		}
	}

	written, err := io.Copy(outFile, reader)
	if err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", destPath, closeErr)
		}

		if removeErr := fs.Remove(destPath); removeErr != nil {
			l.Warnf("Error removing partial file %q: %v", destPath, removeErr)
		}

		return fmt.Errorf("failed to copy file %q: %w", zipFile.Name, err)
	}

	if err := outFile.Close(); err != nil {
		l.Warnf("Error closing file %q: %v", destPath, err)
	}

	// Update total size for limit tracking
	if z.FileSizeLimit > 0 {
		*totalSize += written
	}

	return nil
}

// limitedReader wraps a reader and enforces a size limit.
type limitedReader struct {
	reader    io.Reader
	totalSize *int64
	remaining int64
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, errors.New("decompressed size exceeds limit")
	}

	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)

	return n, err
}

// applyUmask applies a umask to a file mode.
func applyUmask(mode, umask os.FileMode) os.FileMode {
	return mode &^ umask
}
