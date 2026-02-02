// Package vfs provides a virtual filesystem abstraction for testing and production use.
// It wraps afero to provide a consistent interface for filesystem operations.
package vfs

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// Unzip extracts a zip archive from src to dst directory on the given filesystem.
// The umask parameter is applied to file permissions (use 0 to preserve original permissions).
func Unzip(l log.Logger, fs FS, dst, src string, umask os.FileMode) error {
	zipReader, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("failed to open zip archive %q: %w", src, err)
	}

	defer func() {
		if closeErr := zipReader.Close(); closeErr != nil {
			l.Warnf("Error closing zip archive %q: %v", src, closeErr)
		}
	}()

	if err := fs.MkdirAll(dst, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dst, err)
	}

	for _, zipFile := range zipReader.File {
		if err := extractZipFile(l, fs, dst, zipFile, umask); err != nil {
			return fmt.Errorf("failed to extract file %q: %w", zipFile.Name, err)
		}
	}

	return nil
}

// sanitizeZipPath validates and sanitizes a zip entry path to prevent ZipSlip attacks.
func sanitizeZipPath(dst, name string) (string, error) {
	// Check for path traversal attempts
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("illegal file path in zip: %s", name)
	}

	// Clean and join the path
	destPath := filepath.Join(dst, filepath.Clean(name))

	// Verify the path is within the destination directory
	if !strings.HasPrefix(destPath, filepath.Clean(dst)+string(os.PathSeparator)) {
		return "", fmt.Errorf("illegal destination path in zip: %s", destPath)
	}

	return destPath, nil
}

// extractZipFile extracts a single file from a zip archive.
func extractZipFile(l log.Logger, fs FS, dst string, zipFile *zip.File, umask os.FileMode) error {
	destPath, err := sanitizeZipPath(dst, zipFile.Name)
	if err != nil {
		return err
	}

	fileInfo := zipFile.FileInfo()

	// Handle directories
	if fileInfo.IsDir() {
		if err := fs.MkdirAll(destPath, applyUmask(fileInfo.Mode(), umask)); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", destPath, err)
		}

		return nil
	}

	// Handle symlinks
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return extractSymlink(l, fs, destPath, zipFile)
	}

	// Handle regular files
	return extractRegularFile(l, fs, destPath, zipFile, umask)
}

// extractSymlink extracts a symlink from a zip file.
func extractSymlink(l log.Logger, fs FS, destPath string, zipFile *zip.File) error {
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

	// Ensure parent directory exists
	if err := fs.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	return Symlink(fs, target, destPath)
}

// extractRegularFile extracts a regular file from a zip file.
func extractRegularFile(l log.Logger, fs FS, destPath string, zipFile *zip.File, umask os.FileMode) error {
	// Ensure parent directory exists
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

	defer func() {
		if closeErr := outFile.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", destPath, closeErr)
		}
	}()

	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("failed to copy file %q: %w", zipFile.Name, err)
	}

	return nil
}

// applyUmask applies a umask to a file mode.
func applyUmask(mode, umask os.FileMode) os.FileMode {
	return mode &^ umask
}
