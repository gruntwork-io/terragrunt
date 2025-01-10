package util

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	urlhelper "github.com/hashicorp/go-getter/helper/url"

	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mattn/go-zglob"
	homedir "github.com/mitchellh/go-homedir"
)

const (
	TerraformLockFile     = ".terraform.lock.hcl"
	TerragruntCacheDir    = ".terragrunt-cache"
	DefaultBoilerplateDir = ".boilerplate"
	TfFileExtension       = ".tf"
	ChecksumReadBlock     = 8192
)

// FileOrData will read the contents of the data of the given arg if it is a file, and otherwise return the contents by
// itself. This will return an error if the given path is a directory.
func FileOrData(maybePath string) (string, error) {
	// We can blindly pass in maybePath to homedir.Expand, because homedir.Expand only does something if the first
	// character is ~, and if it is, there is a high chance of it being a path instead of data contents.
	expandedMaybePath, err := homedir.Expand(maybePath)
	if err != nil {
		return "", errors.New(err)
	}

	if IsFile(expandedMaybePath) {
		contents, err := os.ReadFile(expandedMaybePath)
		if err != nil {
			return "", errors.New(err)
		}

		return string(contents), nil
	} else if IsDir(expandedMaybePath) {
		return "", errors.New(PathIsNotFile{path: expandedMaybePath})
	}

	return expandedMaybePath, nil
}

// FileExists returns true if the given file exists.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileNotExists returns true if the given file does not exist.
func FileNotExists(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

// EnsureDirectory creates a directory at this path if it does not exist, or error if the path exists and is a file.
func EnsureDirectory(path string) error {
	if FileExists(path) && IsFile(path) {
		return errors.New(PathIsNotDirectory{path})
	} else if !FileExists(path) {
		const ownerReadWriteExecutePerms = 0700

		return errors.New(os.MkdirAll(path, ownerReadWriteExecutePerms))
	}

	return nil
}

// CanonicalPath returns the canonical version of the given path, relative to the given base path. That is, if the given path is a
// relative path, assume it is relative to the given base path. A canonical path is an absolute path with all relative
// components (e.g. "../") fully resolved, which makes it safe to compare paths as strings.
func CanonicalPath(path string, basePath string) (string, error) {
	if !filepath.IsAbs(path) {
		path = JoinPath(basePath, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New(err)
	}

	return CleanPath(absPath), nil
}

// GlobCanonicalPath returns the canonical versions of the given glob paths, relative to the given base path.
func GlobCanonicalPath(basePath string, globPaths ...string) ([]string, error) {
	if len(globPaths) == 0 {
		return []string{}, nil
	}

	var err error

	// Ensure basePath is canonical
	basePath, err = CanonicalPath("", basePath)
	if err != nil {
		return nil, err
	}

	var paths []string

	for _, globPath := range globPaths {
		// Ensure globPath are absolute
		if !filepath.IsAbs(globPath) {
			globPath = filepath.Join(basePath, globPath)
		}

		matches, err := zglob.Glob(globPath)
		if err == nil {
			paths = append(paths, matches...)
		}
	}

	// Make sure all paths are canonical
	for i := range paths {
		paths[i], err = CanonicalPath(paths[i], basePath)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// CanonicalPaths returns the canonical version of the given paths, relative to the given base path. That is, if a given path is a
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

// Grep returns true if the given regex can be found in any of the files matched by the given glob.
func Grep(regex *regexp.Regexp, glob string) (bool, error) {
	// Ideally, we'd use a builin Go library like filepath.Glob here, but per https://github.com/golang/go/issues/11862,
	// the current go implementation doesn't support treating ** as zero or more directories, just zero or one.
	// So we use a third-party library.
	matches, err := zglob.Glob(glob)
	if err != nil {
		return false, errors.New(err)
	}

	for _, match := range matches {
		if IsDir(match) {
			continue
		}

		bytes, err := os.ReadFile(match)
		if err != nil {
			return false, errors.New(err)
		}

		if regex.Match(bytes) {
			return true, nil
		}
	}

	return false, nil
}

// IsDir returns true if the path points to a directory.
func IsDir(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

// IsFile returns true if the path points to a file.
func IsFile(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && !fileInfo.IsDir()
}

// GetPathRelativeTo returns the relative path you would have to take to get from basePath to path.
func GetPathRelativeTo(path string, basePath string) (string, error) {
	if path == "" {
		path = "."
	}

	if basePath == "" {
		basePath = "."
	}

	inputFolderAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", errors.New(err)
	}

	fileAbs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New(err)
	}

	relPath, err := filepath.Rel(inputFolderAbs, fileAbs)
	if err != nil {
		return "", errors.New(err)
	}

	return filepath.ToSlash(relPath), nil
}

// ReadFileAsString returns the contents of the file at the given path as a string.
func ReadFileAsString(path string) (string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Errorf("error reading file at path %s: %w", path, err)
	}

	return string(bytes), nil
}

func listContainsElementWithPrefix(list []string, elementPrefix string) bool {
	for _, element := range list {
		if strings.HasPrefix(element, elementPrefix) {
			return true
		}
	}

	return false
}

func pathContainsPrefix(path string, prefixes []string) bool {
	for _, element := range prefixes {
		if strings.HasPrefix(path, element) {
			return true
		}
	}

	return false
}

// Takes apbsolute glob path and returns an array of expanded relative paths
func expandGlobPath(source, absoluteGlobPath string) ([]string, error) {
	includeExpandedGlobs := []string{}

	absoluteExpandGlob, err := zglob.Glob(absoluteGlobPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// we ignore not exist error as we only care about the globs that exist in the src dir
		return nil, errors.New(err)
	}

	for _, absoluteExpandGlobPath := range absoluteExpandGlob {
		if strings.Contains(absoluteExpandGlobPath, TerragruntCacheDir) {
			continue
		}

		relativeExpandGlobPath, err := GetPathRelativeTo(absoluteExpandGlobPath, source)
		if err != nil {
			return nil, err
		}

		includeExpandedGlobs = append(includeExpandedGlobs, relativeExpandGlobPath)

		if IsDir(absoluteExpandGlobPath) {
			dirExpandGlob, err := expandGlobPath(source, absoluteExpandGlobPath+"/*")
			if err != nil {
				return nil, errors.New(err)
			}

			includeExpandedGlobs = append(includeExpandedGlobs, dirExpandGlob...)
		}
	}

	return includeExpandedGlobs, nil
}

// CopyFolderContents copies the files and folders within the source folder into the destination folder. Note that hidden files and folders
// (those starting with a dot) will be skipped. Will create a specified manifest file that contains paths of all copied files.
func CopyFolderContents(
	logger log.Logger,
	source,
	destination,
	manifestFile string,
	includeInCopy []string,
	excludeFromCopy []string,
) error {
	// Expand all the includeInCopy glob paths, converting the globbed results to relative paths so that they work in
	// the copy filter.
	includeExpandedGlobs := []string{}

	for _, includeGlob := range includeInCopy {
		globPath := filepath.Join(source, includeGlob)

		expandGlob, err := expandGlobPath(source, globPath)
		if err != nil {
			return errors.New(err)
		}

		includeExpandedGlobs = append(includeExpandedGlobs, expandGlob...)
	}

	excludeExpandedGlobs := []string{}

	for _, excludeGlob := range excludeFromCopy {
		globPath := filepath.Join(source, excludeGlob)

		expandGlob, err := expandGlobPath(source, globPath)
		if err != nil {
			return errors.New(err)
		}

		excludeExpandedGlobs = append(excludeExpandedGlobs, expandGlob...)
	}

	return CopyFolderContentsWithFilter(logger, source, destination, manifestFile, func(absolutePath string) bool {
		relativePath, err := GetPathRelativeTo(absolutePath, source)
		pathHasPrefix := pathContainsPrefix(relativePath, excludeExpandedGlobs)

		listHasElementWithPrefix := listContainsElementWithPrefix(includeExpandedGlobs, relativePath)
		if err == nil && listHasElementWithPrefix && !pathHasPrefix {
			return true
		}

		if err == nil && pathContainsPrefix(relativePath, excludeExpandedGlobs) {
			return false
		}

		return !TerragruntExcludes(filepath.FromSlash(relativePath))
	})
}

// CopyFolderContentsWithFilter copies the files and folders within the source folder into the destination folder.
func CopyFolderContentsWithFilter(logger log.Logger, source, destination, manifestFile string, filter func(absolutePath string) bool) error {
	const ownerReadWriteExecutePerms = 0700
	if err := os.MkdirAll(destination, ownerReadWriteExecutePerms); err != nil {
		return errors.New(err)
	}

	manifest := NewFileManifest(logger, destination, manifestFile)
	if err := manifest.Clean(); err != nil {
		return errors.New(err)
	}

	if err := manifest.Create(); err != nil {
		return errors.New(err)
	}

	defer func(manifest *fileManifest) {
		err := manifest.Close()
		if err != nil {
			logger.Warnf("Error closing manifest file: %v", err)
		}
	}(manifest)

	// Why use filepath.Glob here? The original implementation used os.ReadDir, but that method calls lstat on all
	// the files/folders in the directory, including files/folders you may want to explicitly skip. The next attempt
	// was to use filepath.Walk, but that doesn't work because it ignores symlinks. So, now we turn to filepath.Glob.
	files, err := filepath.Glob(source + "/*")
	if err != nil {
		return errors.New(err)
	}

	for _, file := range files {
		fileRelativePath, err := GetPathRelativeTo(file, source)
		if err != nil {
			return err
		}

		if !filter(file) {
			continue
		}

		dest := filepath.Join(destination, fileRelativePath)

		if IsDir(file) {
			info, err := os.Lstat(file)
			if err != nil {
				return errors.New(err)
			}

			if err := os.MkdirAll(dest, info.Mode()); err != nil {
				return errors.New(err)
			}

			if err := CopyFolderContentsWithFilter(logger, file, dest, manifestFile, filter); err != nil {
				return err
			}

			if err := manifest.AddDirectory(dest); err != nil {
				return err
			}
		} else {
			parentDir := filepath.Dir(dest)

			const ownerReadWriteExecutePerms = 0700
			if err := os.MkdirAll(parentDir, ownerReadWriteExecutePerms); err != nil {
				return errors.New(err)
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

func TerragruntExcludes(path string) bool {
	// Do not exclude the terraform lock file (new feature added in terraform 0.14)
	if filepath.Base(path) == TerraformLockFile {
		return false
	}

	pathParts := strings.Split(path, string(filepath.Separator))
	for _, pathPart := range pathParts {
		if strings.HasPrefix(pathPart, ".") && pathPart != "." && pathPart != ".." {
			return true
		}
	}

	return false
}

// CopyFile copies a file from source to destination.
func CopyFile(source string, destination string) error {
	contents, err := os.ReadFile(source)
	if err != nil {
		return errors.New(err)
	}

	return WriteFileWithSamePermissions(source, destination, contents)
}

// WriteFileWithSamePermissions writes a file to the given destination with the given contents
// using the same permissions as the file at source.
func WriteFileWithSamePermissions(source string, destination string, contents []byte) error {
	fileInfo, err := os.Stat(source)
	if err != nil {
		return errors.New(err)
	}

	return os.WriteFile(destination, contents, fileInfo.Mode())
}

// JoinPath is a wrapper around filepath.Join
//
// Windows systems use \ as the path separator *nix uses /
// Use this function when joining paths to force the returned path to use / as the path separator
// This will improve cross-platform compatibility
func JoinPath(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// SplitPath splits the given path into a list.
// E.g. "foo/bar/boo.txt" -> ["foo", "bar", "boo.txt"]
// E.g. "/foo/bar/boo.txt" -> ["", "foo", "bar", "boo.txt"]
// Notice that if path is absolute the resulting list will begin with an empty string.
func SplitPath(path string) []string {
	return strings.Split(CleanPath(path), filepath.ToSlash(string(filepath.Separator)))
}

// CleanPath is a wrapper around filepath.Clean.
//
// Use this function when cleaning paths to ensure the returned
// path uses / as the path separator to improve cross-platform compatibility
func CleanPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

// ContainsPath returns true if path contains the given subpath
// E.g. path="foo/bar/bee", subpath="bar/bee" -> true
// E.g. path="foo/bar/bee", subpath="bar/be" -> false (because be is not a directory)
func ContainsPath(path, subpath string) bool {
	splitPath := SplitPath(CleanPath(path))
	splitSubpath := SplitPath(CleanPath(subpath))
	contains := ListContainsSublist(splitPath, splitSubpath)

	return contains
}

// HasPathPrefix returns true if path starts with the given path prefix
// E.g. path="/foo/bar/biz", prefix="/foo/bar" -> true
// E.g. path="/foo/bar/biz", prefix="/foo/ba" -> false (because ba is not a directory
// path)
func HasPathPrefix(path, prefix string) bool {
	splitPath := SplitPath(CleanPath(path))
	splitPrefix := SplitPath(CleanPath(prefix))
	hasPrefix := ListHasPrefix(splitPath, splitPrefix)

	return hasPrefix
}

// JoinTerraformModulePath joins two paths together with a double-slash between them, as this is what
// Terraform uses to identify where a "repo" ends and a path within the repo begins.
// Note: The Terraform docs only mention two forward-slashes, so it's not clear
// if on Windows those should be two back-slashes? https://www.terraform.io/docs/modules/sources.html
func JoinTerraformModulePath(modulesFolder string, path string) string {
	cleanModulesFolder := strings.TrimRight(modulesFolder, `/\`)
	cleanPath := strings.TrimLeft(path, `/\`)
	// if source path contains "?ref=", reconstruct module dir using "//"
	if strings.Contains(cleanModulesFolder, "?ref=") && cleanPath != "" {
		canonicalSourceURL, err := urlhelper.Parse(cleanModulesFolder)
		if err == nil {
			// append path
			if canonicalSourceURL.Opaque != "" {
				canonicalSourceURL.Opaque = fmt.Sprintf("%s//%s", strings.TrimRight(canonicalSourceURL.Opaque, `/\`), cleanPath)
			} else {
				canonicalSourceURL.Path = fmt.Sprintf("%s//%s", strings.TrimRight(canonicalSourceURL.Path, `/\`), cleanPath)
			}

			return canonicalSourceURL.String()
		}
	}

	// fallback to old behavior if we can't parse the url
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
	logger         log.Logger
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

	// cleaning manifest file
	defer func(name string) {
		if err := file.Close(); err != nil {
			manifest.logger.Warnf("Error closing file %s: %v", name, err)
		}

		if err := os.Remove(name); err != nil {
			manifest.logger.Warnf("Error removing manifest file %s: %v", name, err)
		}
	}(manifestPath)

	decoder := gob.NewDecoder(file)
	// decode paths one by one
	for {
		var manifestEntry fileManifestEntry

		err = decoder.Decode(&manifestEntry)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				return err
			}
		}

		if manifestEntry.IsDir {
			// join the directory entry path with the manifest file name and call clean()
			if err := manifest.clean(filepath.Join(manifestEntry.Path, manifest.ManifestFile)); err != nil {
				return errors.New(err)
			}
		} else {
			if err := os.Remove(manifestEntry.Path); err != nil && !os.IsNotExist(err) {
				return errors.New(err)
			}
		}
	}

	return nil
}

// Create will create the manifest file
func (manifest *fileManifest) Create() error {
	const ownerWriteGlobalReadPerms = 0644

	fileHandle, err := os.OpenFile(filepath.Join(manifest.ManifestFolder, manifest.ManifestFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, ownerWriteGlobalReadPerms)
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

func NewFileManifest(logger log.Logger, manifestFolder string, manifestFile string) *fileManifest {
	return &fileManifest{logger: logger, ManifestFolder: manifestFolder, ManifestFile: manifestFile}
}

// Custom errors

// PathIsNotDirectory is returned when the given path is unexpectedly not a directory.
type PathIsNotDirectory struct {
	path string
}

func (err PathIsNotDirectory) Error() string {
	return err.path + " is not a directory"
}

// PathIsNotFile is returned when the given path is unexpectedly not a file.
type PathIsNotFile struct {
	path string
}

func (err PathIsNotFile) Error() string {
	return err.path + " is not a file"
}

// ListTfFiles returns a list of all TF files in the specified directory.
func ListTfFiles(directoryPath string, walkWithSymlinks bool) ([]string, error) {
	var tfFiles []string

	walkFunc := filepath.Walk
	if walkWithSymlinks {
		walkFunc = WalkWithSymlinks
	}

	err := walkFunc(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == TfFileExtension {
			tfFiles = append(tfFiles, path)
		}

		return nil
	})

	return tfFiles, err
}

// IsDirectoryEmpty - returns true if the given path exists and is a empty directory.
func IsDirectoryEmpty(dirPath string) (bool, error) {
	dir, err := os.Open(dirPath)
	if err != nil {
		return false, err
	}

	defer func() {
		_ = dir.Close()
	}()

	_, err = dir.Readdir(1)
	if err == nil {
		return false, nil
	}

	return true, nil
}

// GetCacheDir returns the global terragrunt cache directory for the current user.
func GetCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New(err)
	}

	cacheDir = filepath.Join(cacheDir, "terragrunt")

	if !FileExists(cacheDir) {
		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
			return "", errors.New(err)
		}
	}

	return cacheDir, nil
}

// GetTempDir returns the global terragrunt temp directory.
func GetTempDir() (string, error) {
	tempDir := filepath.Join(os.TempDir(), "terragrunt")

	if !FileExists(tempDir) {
		if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
			return "", errors.New(err)
		}
	}

	return tempDir, nil
}

// GetExcludeDirsFromFile returns a list of directories from the given filename, where each directory path starts on a new line.
func GetExcludeDirsFromFile(baseDir, filename string) ([]string, error) {
	filename, err := CanonicalPath(filename, baseDir)
	if err != nil {
		return nil, err
	}

	if !FileExists(filename) || !IsFile(filename) {
		return nil, nil
	}

	content, err := ReadFileAsString(filename)
	if err != nil {
		return nil, err
	}

	var dirs []string

	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for _, dir := range lines {
		if dir := strings.TrimSpace(dir); dir == "" || strings.HasPrefix(dir, "#") {
			continue
		}

		newDirs, err := GlobCanonicalPath(baseDir, dir)
		if err != nil {
			return nil, err
		}

		dirs = append(dirs, newDirs...)
	}

	return dirs, nil
}

// MatchSha256Checksum returns the SHA256 checksum for the given file and filename.
func MatchSha256Checksum(file, filename []byte) []byte {
	var checksum []byte

	for _, line := range bytes.Split(file, []byte("\n")) {
		parts := bytes.Fields(line)
		if len(parts) > 1 && bytes.Equal(parts[1], filename) {
			checksum = parts[0]
			break
		}
	}

	if checksum == nil {
		return nil
	}

	return checksum
}

// FileSHA256 calculates the SHA256 hash of the file at the given path.
func FileSHA256(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, errors.New(err)
	}
	defer file.Close() //nolint:errcheck

	hash := sha256.New()
	buffer := make([]byte, ChecksumReadBlock)

	for {
		n, err := file.Read(buffer)
		if err != nil && err != io.EOF {
			return nil, errors.New(err)
		}

		if n == 0 {
			break
		}

		if _, err := hash.Write(buffer[:n]); err != nil {
			return nil, errors.New(err)
		}
	}

	return hash.Sum(nil), nil
}

// readerFunc is syntactic sugar for read interface.
type readerFunc func(data []byte) (int, error)

func (rf readerFunc) Read(data []byte) (int, error) { return rf(data) }

// writerFunc is syntactic sugar for write interface.
type writerFunc func(data []byte) (int, error)

func (wf writerFunc) Write(data []byte) (int, error) { return wf(data) }

// Copy is a io.Copy cancellable by context.
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	num, err := io.Copy(
		writerFunc(func(data []byte) (int, error) {
			select {
			case <-ctx.Done():
				// context has been canceled stop process and propagate "context canceled" error.
				return 0, ctx.Err()
			default:
				// otherwise just run default io.Writer implementation.
				return dst.Write(data)
			}
		}),
		readerFunc(func(data []byte) (int, error) {
			select {
			case <-ctx.Done():
				// context has been canceled stop process and propagate "context canceled" error.
				return 0, ctx.Err()
			default:
				// otherwise just run default io.Reader implementation.
				return src.Read(data)
			}
		}),
	)

	if err != nil {
		err = errors.New(err)
	}

	return num, err
}

// WalkWithSymlinks traverses a directory tree, following symbolic links and calling
// the provided function for each file or directory encountered. It handles both regular
// symlinks and circular symlinks without getting into infinite loops.
//
//nolint:funlen
func WalkWithSymlinks(root string, externalWalkFn filepath.WalkFunc) error {
	// pathPair keeps track of both the physical (real) path on disk
	// and the logical path (how it appears in the walk)
	type pathPair struct {
		physical string
		logical  string
	}

	// visited tracks symlink paths to prevent circular references
	// key is combination of realPath:symlinkPath
	visited := make(map[string]bool)

	// visitedLogical tracks logical paths to prevent duplicates
	// when the same directory is reached through different symlinks
	visitedLogical := make(map[string]bool)

	var walkFn func(pathPair) error

	walkFn = func(pair pathPair) error {
		return filepath.Walk(pair.physical, func(currentPath string, info os.FileInfo, err error) error {
			if err != nil {
				return externalWalkFn(currentPath, info, err)
			}

			// Convert the current physical path to a logical path relative to the walk root
			rel, err := filepath.Rel(pair.physical, currentPath)
			if err != nil {
				return fmt.Errorf("failed to get relative path between %s and %s: %w", pair.physical, currentPath, err)
			}

			logicalPath := filepath.Join(pair.logical, rel)

			realPath, realInfo, err := evalRealPathAndInfo(currentPath)
			if err != nil {
				return err
			}

			// Call the provided function only if we haven't seen this logical path before
			if !visitedLogical[logicalPath] {
				visitedLogical[logicalPath] = true

				if err := externalWalkFn(logicalPath, realInfo, nil); err != nil {
					return err
				}
			}

			// If we encounter a symlink, resolve and follow it
			if info.Mode()&os.ModeSymlink != 0 {
				// Skip if we've seen this symlink->target combination before
				// This prevents infinite loops with circular symlinks
				if visited[realPath+":"+currentPath] {
					return nil
				}

				visited[realPath+":"+currentPath] = true

				// If the target is a directory, recursively walk it
				if realInfo.IsDir() {
					return walkFn(pathPair{
						physical: realPath,
						logical:  logicalPath,
					})
				}
			}

			return nil
		})
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("failed to get evaluate sym links for %s: %w", root, err)
	}

	// Start the walk from the root directory
	return walkFn(pathPair{
		physical: realRoot,
		logical:  realRoot,
	})
}

func evalRealPathAndInfo(currentPath string) (string, os.FileInfo, error) {
	realPath, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get evaluate sym links for %s: %w", currentPath, err)
	}

	// Get info about the symlink target
	realInfo, err := os.Stat(realPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to describe file %s: %w", realPath, err)
	}

	return realPath, realInfo, nil
}
