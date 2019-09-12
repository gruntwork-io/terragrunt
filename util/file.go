package util

import (
	"encoding/gob"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fmt"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/mattn/go-zglob"
)

// Return true if the given file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Return the canonical version of the given path, relative to the given base path. That is, if the given path is a
// relative path, assume it is relative to the given base path. A canonical path is an absolute path with all relative
// components (e.g. "../") fully resolved, which makes it safe to compare paths as strings.
func CanonicalPath(path string, basePath string) (string, error) {
	if !filepath.IsAbs(path) {
		path = JoinPath(basePath, path)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return CleanPath(absPath), nil
}

// Return the canonical version of the given paths, relative to the given base path. That is, if a given path is a
// relative path, assume it is relative to the given base path. A canonical path is an absolute path with all relative
// components (e.g. "../") fully resolved, which makes it safe to compare paths as strings.
func CanonicalPaths(paths []string, basePath string) ([]string, error) {
	canonicalPaths := []string{}

	for _, path := range paths {
		canonicalPath, err := CanonicalPath(path, basePath)
		if err != nil {
			return canonicalPaths, err
		}
		canonicalPaths = append(canonicalPaths, canonicalPath)
	}

	return canonicalPaths, nil
}

// Returns true if the given regex can be found in any of the files matched by the given glob
func Grep(regex *regexp.Regexp, glob string) (bool, error) {
	// Ideally, we'd use a builin Go library like filepath.Glob here, but per https://github.com/golang/go/issues/11862,
	// the current go implementation doesn't support treating ** as zero or more directories, just zero or one.
	// So we use a third-party library.
	matches, err := zglob.Glob(glob)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	for _, match := range matches {
		if IsDir(match) {
			continue
		}
		bytes, err := ioutil.ReadFile(match)
		if err != nil {
			return false, errors.WithStackTrace(err)
		}

		if regex.Match(bytes) {
			return true, nil
		}
	}

	return false, nil
}

// Return true if the path points to a directory
func IsDir(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

// Return true if the path points to a file
func IsFile(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && !fileInfo.IsDir()
}

// Return the relative path you would have to take to get from basePath to path
func GetPathRelativeTo(path string, basePath string) (string, error) {
	if path == "" {
		path = "."
	}
	if basePath == "" {
		basePath = "."
	}

	inputFolderAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	fileAbs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	relPath, err := filepath.Rel(inputFolderAbs, fileAbs)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(relPath), nil
}

// Return the contents of the file at the given path as a string
func ReadFileAsString(path string) (string, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errors.WithStackTraceAndPrefix(err, "Error reading file at path %s", path)
	}

	return string(bytes), nil
}

// Copy the files and folders within the source folder into the destination folder. Note that hidden files and folders
// (those starting with a dot) will be skipped. Will create a specified manifest file that contains paths of all copied files.
func CopyFolderContents(source, destination, manifestFile string) error {
	return CopyFolderContentsWithFilter(source, destination, manifestFile, func(path string) bool {
		return !PathContainsHiddenFileOrFolder(path)
	})
}

// Copy the files and folders within the source folder into the destination folder. Pass each file and folder through
// the given filter function and only copy it if the filter returns true. Will create a specified manifest file
// that contains paths of all copied files.
func CopyFolderContentsWithFilter(source, destination, manifestFile string, filter func(path string) bool) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return errors.WithStackTrace(err)
	}
	manifest := newFileManifest(destination, manifestFile)
	if err := manifest.Clean(); err != nil {
		return errors.WithStackTrace(err)
	}
	if err := manifest.Create(); err != nil {
		return errors.WithStackTrace(err)
	}
	defer manifest.Close()

	// Why use filepath.Glob here? The original implementation used ioutil.ReadDir, but that method calls lstat on all
	// the files/folders in the directory, including files/folders you may want to explicitly skip. The next attempt
	// was to use filepath.Walk, but that doesn't work because it ignores symlinks. So, now we turn to filepath.Glob.
	files, err := filepath.Glob(fmt.Sprintf("%s/*", source))
	if err != nil {
		return errors.WithStackTrace(err)
	}

	for _, file := range files {
		fileRelativePath, err := GetPathRelativeTo(file, source)
		if err != nil {
			return err
		}

		if !filter(fileRelativePath) {
			continue
		}

		dest := filepath.Join(destination, fileRelativePath)

		if IsDir(file) {
			info, err := os.Lstat(file)
			if err != nil {
				return errors.WithStackTrace(err)
			}

			if err := os.MkdirAll(dest, info.Mode()); err != nil {
				return errors.WithStackTrace(err)
			}

			if err := CopyFolderContentsWithFilter(file, dest, manifestFile, filter); err != nil {
				return err
			}
			if err := manifest.AddDirectory(dest); err != nil {
				return err
			}
		} else {
			parentDir := filepath.Dir(dest)
			if err := os.MkdirAll(parentDir, 0700); err != nil {
				return errors.WithStackTrace(err)
			}
			if err := CopyFile(file, dest); err != nil {
				return err
			}
			if err := manifest.AddFile(dest); err != nil {
				return err
			}
		}
	}

	return nil
}

// IsSymLink returns true if the given file is a symbolic link
// Per https://stackoverflow.com/a/18062079/2308858
func IsSymLink(path string) bool {
	fileInfo, err := os.Lstat(path)
	return err == nil && fileInfo.Mode()&os.ModeSymlink != 0
}

func PathContainsHiddenFileOrFolder(path string) bool {
	pathParts := strings.Split(path, string(filepath.Separator))
	for _, pathPart := range pathParts {
		if strings.HasPrefix(pathPart, ".") && pathPart != "." && pathPart != ".." {
			return true
		}
	}
	return false
}

// Copy a file from source to destination
func CopyFile(source string, destination string) error {
	contents, err := ioutil.ReadFile(source)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	return WriteFileWithSamePermissions(source, destination, contents)
}

// Write a file to the given destination with the given contents using the same permissions as the file at source
func WriteFileWithSamePermissions(source string, destination string, contents []byte) error {
	fileInfo, err := os.Stat(source)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	return ioutil.WriteFile(destination, contents, fileInfo.Mode())
}

// Windows systems use \ as the path separator *nix uses /
// Use this function when joining paths to force the returned path to use / as the path separator
// This will improve cross-platform compatibility
func JoinPath(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// Use this function when cleaning paths to ensure the returned path uses / as the path separator to improve cross-platform compatibility
func CleanPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

// Join two paths together with a double-slash between them, as this is what Terraform uses to identify where a "repo"
// ends and a path within the repo begins. Note: The Terraform docs only mention two forward-slashes, so it's not clear
// if on Windows those should be two back-slashes? https://www.terraform.io/docs/modules/sources.html
func JoinTerraformModulePath(modulesFolder string, path string) string {
	cleanModulesFolder := strings.TrimRight(modulesFolder, `/\`)
	cleanPath := strings.TrimLeft(path, `/\`)
	return fmt.Sprintf("%s//%s", cleanModulesFolder, cleanPath)
}

// fileManifest represents a manifest with paths of all files copied by terragrunt.
// This allows to clean those files on subsequent runs.
// The problem is as follows: terragrunt copies the terraform source code first to "working directory" using go-getter,
// and then copies all files from the working directory to the above dir.
// It works fine on the first run, but if we delete a file from the current terragrunt directory, we want it
// to be cleaned in the "working directory" as well. Since we don't really know what can get copied by go-getter,
// we have to track all the files we touch in a manifest. This way we know exactly which files we need to clean on
// subsequent runs.
type fileManifest struct {
	ManifestFolder string // this is a folder that has the manifest in it
	ManifestFile   string // this is the manifest file name
	encoder        *gob.Encoder
	fileHandle     *os.File
}

// fileManifestEntry represents an entry in the fileManifest.
// It uses a struct with IsDir flag so that we won't have to call Stat on every
// file to determine if it's a directory or a file
type fileManifestEntry struct {
	Path  string
	IsDir bool
}

// Clean will recursively remove all files specified in the manifest
func (manifest *fileManifest) Clean() error {
	return manifest.clean(filepath.Join(manifest.ManifestFolder, manifest.ManifestFile))
}

// clean cleans the files in the manifest. If it has a directory entry, then it recursively calls clean()
func (manifest *fileManifest) clean(manifestPath string) error {
	// if manifest file doesn't exist, just exit
	if !FileExists(manifestPath) {
		return nil
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := gob.NewDecoder(file)
	// decode paths one by one
	for {
		var manifestEntry fileManifestEntry
		err = decoder.Decode(&manifestEntry)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				return err
			}
		}
		if manifestEntry.IsDir {
			// join the directory entry path with the manifest file name and call clean()
			if err := manifest.clean(filepath.Join(manifestEntry.Path, manifest.ManifestFile)); err != nil {
				return errors.WithStackTrace(err)
			}
		} else {
			if err := os.Remove(manifestEntry.Path); err != nil && !os.IsNotExist(err) {
				return errors.WithStackTrace(err)
			}
		}
	}
	// remove the manifest itself
	// it will run after the close defer
	defer os.Remove(manifestPath)

	return nil
}

// Create will create the manifest file
func (manifest *fileManifest) Create() error {
	fileHandle, err := os.OpenFile(filepath.Join(manifest.ManifestFolder, manifest.ManifestFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	manifest.fileHandle = fileHandle
	manifest.encoder = gob.NewEncoder(manifest.fileHandle)

	return nil
}

// AddFile will add the file path to the manifest file. Please make sure to run Create() before using this
func (manifest *fileManifest) AddFile(path string) error {
	return manifest.encoder.Encode(fileManifestEntry{Path: path, IsDir: false})
}

// AddDirectory will add the directory path to the manifest file. Please make sure to run Create() before using this
func (manifest *fileManifest) AddDirectory(path string) error {
	return manifest.encoder.Encode(fileManifestEntry{Path: path, IsDir: true})
}

// Close closes the manifest file handle
func (manifest *fileManifest) Close() error {
	return manifest.fileHandle.Close()
}

func newFileManifest(manifestFolder string, manifestFile string) *fileManifest {
	return &fileManifest{ManifestFolder: manifestFolder, ManifestFile: manifestFile}
}
