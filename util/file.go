package util

import (
	"os"
	"path/filepath"
	"github.com/gruntwork-io/terragrunt/errors"
	"io/ioutil"
	"regexp"
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
	matches, err := filepath.Glob(glob)
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	for _, match := range matches {
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

// Copy the files and folders within the source folder into the destination folder
func CopyFolderContents(source string, destination string) error {
	files, err := ioutil.ReadDir(source)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	for _, file := range files {
		src := filepath.Join(source, file.Name())
		dest := filepath.Join(destination, file.Name())

		if file.IsDir() {
			if err := os.MkdirAll(dest, file.Mode()); err != nil {
				return errors.WithStackTrace(err)
			}

			if err := CopyFolderContents(src, dest); err != nil {
				return err
			}
		} else {
			if err := CopyFile(src, dest); err != nil {
				return err
			}
		}
	}

	return nil
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