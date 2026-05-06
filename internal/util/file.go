package util

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"

	urlhelper "github.com/hashicorp/go-getter/helper/url"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mitchellh/go-homedir"
)

const (
	TerraformLockFile     = ".terraform.lock.hcl"
	TerragruntCacheDir    = ".terragrunt-cache"
	TerraformCacheDir     = ".terraform"
	GitDir                = ".git"
	DefaultBoilerplateDir = ".boilerplate"
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
	return errors.Is(err, fs.ErrNotExist)
}

// EnsureDirectory creates a directory at this path if it does not exist, or error if the path exists and is a file.
func EnsureDirectory(path string) error {
	if FileExists(path) && IsFile(path) {
		return errors.New(PathIsNotDirectory{path})
	} else if !FileExists(path) {
		const ownerReadWriteExecutePerms = 0o700

		return errors.New(os.MkdirAll(path, ownerReadWriteExecutePerms))
	}

	return nil
}

// CanonicalPath returns the canonical version of the given path, relative to the given base path. That is, if the given
// path is a relative path, assume it is relative to the given base path. A canonical path is an absolute path with all
// relative components (e.g. "../") fully resolved, which makes it safe to compare paths as strings. If the path is
// relative, basePath must be absolute or an error is returned.
func CanonicalPath(path string, basePath string) (string, error) {
	if !filepath.IsAbs(path) {
		if !filepath.IsAbs(basePath) {
			return "", fmt.Errorf("base path %q is not absolute", basePath)
		}

		path = filepath.Join(basePath, path)
	}

	return filepath.Clean(path), nil
}

// CanonicalResolvedPath returns the cleaned absolute path with symlinks resolved best-effort.
func CanonicalResolvedPath(path, basePath string) (string, error) {
	canonical, err := CanonicalPath(path, basePath)
	if err != nil {
		return "", err
	}

	return ResolvePath(canonical), nil
}

// GrepFilesWithSuffix returns true if regex matches the contents of any file
// under rootDir whose name ends with suffix. The walk stops as soon as a match
// is found. A missing rootDir is not an error; the function returns false.
func GrepFilesWithSuffix(fsys vfs.FS, regex *regexp.Regexp, rootDir, suffix string) (bool, error) {
	var found bool

	err := vfs.WalkDir(fsys, rootDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return fs.SkipAll
			}

			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(d.Name(), suffix) {
			return nil
		}

		contents, err := vfs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		if regex.Match(contents) {
			found = true
			return fs.SkipAll
		}

		return nil
	})
	if err != nil {
		return false, errors.New(err)
	}

	return found, nil
}

// FindTFFiles walks through the directory and returns all OpenTofu/Terraform files (.tf, .tofu, .tf.json, .tofu.json)
func FindTFFiles(rootPath string) ([]string, error) {
	var terraformFiles []string

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if IsTFFile(path) {
			terraformFiles = append(terraformFiles, path)
		}

		return nil
	})

	return terraformFiles, err
}

// RegexFoundInTFFiles walks through the directory and checks if any OpenTofu/Terraform files (.tf, .tofu, .tf.json, .tofu.json) contain the given regex pattern
func RegexFoundInTFFiles(workingDir string, pattern *regexp.Regexp) (bool, error) {
	var found bool

	err := filepath.WalkDir(workingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !IsTFFile(path) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if pattern.Match(content) {
			found = true
			return filepath.SkipAll
		}

		return nil
	})

	return found, err
}

// DirContainsTFFiles checks if the given directory contains any Terraform/OpenTofu files (.tf, .tofu, .tf.json, .tofu.json)
func DirContainsTFFiles(dirPath string) (bool, error) {
	var found bool

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if IsTFFile(path) {
			found = true
			return filepath.SkipAll
		}

		return nil
	})

	return found, err
}

// IsTFFile checks if a given file is a Terraform/OpenTofu file (.tf, .tofu, .tf.json, .tofu.json)
func IsTFFile(path string) bool {
	suffixes := []string{
		".tf",
		".tofu",
		".tf.json",
		".tofu.json",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
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

	absoluteExpandGlob, err := glob.LegacyExpand(absoluteGlobPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// we ignore not exist error as we only care about the globs that exist in the src dir
		return nil, errors.New(err)
	}

	for _, absoluteExpandGlobPath := range absoluteExpandGlob {
		if strings.Contains(absoluteExpandGlobPath, TerragruntCacheDir) {
			continue
		}

		relativeExpandGlobPath, err := filepath.Rel(source, absoluteExpandGlobPath)
		if err != nil {
			return nil, fmt.Errorf("relativize glob match %q against source %q: %w", absoluteExpandGlobPath, source, err)
		}

		includeExpandedGlobs = append(includeExpandedGlobs, filepath.ToSlash(relativeExpandGlobPath))

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

// CopyOption configures a [CopyFolderContents] call.
type CopyOption func(*copyConfig)

type copyConfig struct {
	includeInCopy   []string
	excludeFromCopy []string
	fastCopy        bool
}

// WithIncludeInCopy adds glob patterns that must be copied even when
// [TerragruntExcludes] would skip them (for example hidden files).
func WithIncludeInCopy(patterns ...string) CopyOption {
	return func(c *copyConfig) {
		c.includeInCopy = append(c.includeInCopy, patterns...)
	}
}

// WithExcludeFromCopy adds glob patterns whose matches must be skipped
// during the copy.
func WithExcludeFromCopy(patterns ...string) CopyOption {
	return func(c *copyConfig) {
		c.excludeFromCopy = append(c.excludeFromCopy, patterns...)
	}
}

// WithFastCopy enables the fast-copy path: patterns compile once through
// [glob.Compile] and the source tree is walked once via
// [vfs.WalkDirParallel]. See the `fast-copy` strict control for the
// semantic implications.
func WithFastCopy() CopyOption {
	return func(c *copyConfig) {
		c.fastCopy = true
	}
}

// CopyFolderContents copies the files and folders within the source folder into the destination folder. Note that hidden files and folders
// (those starting with a dot) will be skipped. Will create a specified manifest file that contains paths of all copied files.
//
// Optional behavior is configured through [CopyOption] values such as
// [WithIncludeInCopy], [WithExcludeFromCopy], and [WithFastCopy].
func CopyFolderContents(l log.Logger, source, destination, manifestFile string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	// We use filepath.ToSlash because we end up using globs here, and those expect forward slashes.
	source = filepath.ToSlash(source)
	destination = filepath.ToSlash(destination)

	if cfg.fastCopy {
		return copyFolderContentsFast(l, source, destination, manifestFile, cfg.includeInCopy, cfg.excludeFromCopy)
	}

	// Expand all the includeInCopy glob paths, converting the globbed results to relative paths so that they work in
	// the copy filter.
	includeExpandedGlobs := []string{}

	for _, includeGlob := range cfg.includeInCopy {
		globPath := filepath.Join(source, includeGlob)

		expandGlob, err := expandGlobPath(source, globPath)
		if err != nil {
			return errors.New(err)
		}

		includeExpandedGlobs = append(includeExpandedGlobs, expandGlob...)
	}

	excludeExpandedGlobs := []string{}

	for _, excludeGlob := range cfg.excludeFromCopy {
		globPath := filepath.Join(source, excludeGlob)

		expandGlob, err := expandGlobPath(source, globPath)
		if err != nil {
			return errors.New(err)
		}

		excludeExpandedGlobs = append(excludeExpandedGlobs, expandGlob...)
	}

	return CopyFolderContentsWithFilter(l, source, destination, manifestFile, func(absolutePath string) bool {
		relativePath, err := filepath.Rel(source, absolutePath)
		if err != nil {
			return false
		}

		relativePath = filepath.ToSlash(relativePath)
		pathHasPrefix := pathContainsPrefix(relativePath, excludeExpandedGlobs)

		listHasElementWithPrefix := listContainsElementWithPrefix(includeExpandedGlobs, relativePath)
		if listHasElementWithPrefix && !pathHasPrefix {
			return true
		}

		if pathHasPrefix {
			return false
		}

		return !TerragruntExcludes(filepath.FromSlash(relativePath))
	})
}

// copyFolderContentsFast is the [CopyFolderContents] path used when the
// `fast-copy` strict control is enabled. Include and exclude patterns
// are compiled once and the source tree is walked once through
// [vfs.WalkDirParallel].
func copyFolderContentsFast(
	l log.Logger,
	source,
	destination,
	manifestFile string,
	includeInCopy []string,
	excludeFromCopy []string,
) error {
	include, err := compileIncludePatterns(includeInCopy)
	if err != nil {
		return err
	}

	exclude, err := compileExcludePattern(excludeFromCopy)
	if err != nil {
		return err
	}

	const ownerReadWriteExecutePerms = 0o700
	if err := os.MkdirAll(destination, ownerReadWriteExecutePerms); err != nil {
		return errors.New(err)
	}

	manifest := NewFileManifest(destination, manifestFile)
	if err := manifest.Clean(l); err != nil {
		return errors.New(err)
	}

	if err := manifest.Create(); err != nil {
		return errors.New(err)
	}

	defer func() {
		if err := manifest.Close(); err != nil {
			l.Warnf("Error closing manifest file: %v", err)
		}
	}()

	// The walk is parallel. The gob-encoded manifest is not safe for
	// concurrent writes, so AddFile is guarded.
	var manifestMu sync.Mutex

	walkFn := func(absolutePath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if absolutePath == source {
			return nil
		}

		rel, err := filepath.Rel(source, absolutePath)
		if err != nil {
			return errors.New(err)
		}

		rel = filepath.ToSlash(rel)

		isDir := d.IsDir()

		// fastwalk reports the link itself (type = symlink) before
		// descending into a followed directory symlink, so `d.IsDir()`
		// is false on the initial visit. Stat the target so the rest of
		// the walkFn treats a directory symlink as a directory and
		// matches the legacy copy semantics.
		if !isDir && d.Type()&fs.ModeSymlink != 0 {
			targetInfo, err := os.Stat(absolutePath)
			if err != nil {
				return errors.New(err)
			}

			isDir = targetInfo.IsDir()
		}

		// Skip .terragrunt-cache before include matching. A user
		// include like "**" would otherwise pull it back in.
		if slices.Contains(strings.Split(rel, "/"), TerragruntCacheDir) {
			if isDir {
				return fs.SkipDir
			}

			return nil
		}

		if exclude != nil && exclude.Match(rel) {
			if isDir {
				return fs.SkipDir
			}

			return nil
		}

		included := include.matches(rel)

		if !included && TerragruntExcludes(filepath.FromSlash(rel)) {
			// A directory on the way to a potential include match must
			// still be descended into even when TerragruntExcludes
			// would reject it.
			if isDir && include.isAncestor(rel) {
				return nil
			}

			if isDir {
				return fs.SkipDir
			}

			return nil
		}

		dest := filepath.Join(destination, rel)

		if isDir {
			info, err := d.Info()
			if err != nil {
				return errors.New(err)
			}

			// A sibling file-copy worker may have created `dest`
			// already with default perms. Chmod forces the source's
			// mode.
			if err := os.MkdirAll(dest, info.Mode().Perm()); err != nil {
				return errors.New(err)
			}

			if err := os.Chmod(dest, info.Mode().Perm()); err != nil {
				return errors.New(err)
			}

			return nil
		}

		parentDir := filepath.Dir(dest)
		if err := os.MkdirAll(parentDir, ownerReadWriteExecutePerms); err != nil {
			return errors.New(err)
		}

		if err := CopyFile(absolutePath, dest); err != nil {
			return err
		}

		manifestMu.Lock()
		defer manifestMu.Unlock()

		return manifest.AddFile(dest)
	}

	if err := vfs.WalkDirParallel(vfs.NewOSFS(), source, walkFn, vfs.WithFollowSymlinks()); err != nil {
		return errors.New(err)
	}

	return nil
}

// includePatterns holds the compiled include matcher and an ancestor
// predicate. The predicate lets the walk descend through dot-prefixed
// parents like `_module/.region3` to reach an include below.
type includePatterns struct {
	// match is a single matcher that OR-s every user pattern as
	// `{<p>,<p>/**}` so one Match call per entry covers all of them.
	match glob.Matcher

	// ancestor matches the path prefixes of every pattern that does
	// not contain `**`. A rel that matches is a directory on the way
	// to a potential include match.
	ancestor glob.Matcher

	// descendAny is true when any pattern contains a `**` segment. In
	// that case every directory is a possible ancestor, so `ancestor`
	// is not consulted.
	descendAny bool
}

func (p includePatterns) matches(rel string) bool {
	return p.match != nil && p.match.Match(rel)
}

func (p includePatterns) isAncestor(rel string) bool {
	if rel == "" || rel == "." {
		return true
	}

	if p.descendAny {
		return true
	}

	return p.ancestor != nil && p.ancestor.Match(rel)
}

// compileIncludePatterns compiles user `include_in_copy` patterns into
// one combined matcher that covers each pattern and all its descendants,
// reproducing the recursive expansion in [expandGlobPath], plus an
// ancestor matcher for directories on the path toward a potential match.
func compileIncludePatterns(patterns []string) (includePatterns, error) {
	out := includePatterns{}

	if len(patterns) == 0 {
		return out, nil
	}

	// Each pattern contributes two alternatives: itself and itself/**.
	const altsPerPattern = 2

	matchParts := make([]string, 0, altsPerPattern*len(patterns))

	var ancestorParts []string

	for _, p := range patterns {
		normalized := strings.TrimRight(filepath.ToSlash(p), "/")
		if normalized == "" {
			continue
		}

		matchParts = append(matchParts, normalized, normalized+"/**")

		segments := strings.Split(normalized, "/")

		if slices.Contains(segments, "**") {
			out.descendAny = true
			continue
		}

		for i := 1; i < len(segments); i++ {
			ancestorParts = append(ancestorParts, strings.Join(segments[:i], "/"))
		}
	}

	match, err := glob.Compile("{" + strings.Join(matchParts, ",") + "}")
	if err != nil {
		return includePatterns{}, errors.New(err)
	}

	out.match = match

	if len(ancestorParts) > 0 {
		ancestor, err := glob.Compile("{" + strings.Join(ancestorParts, ",") + "}")
		if err != nil {
			return includePatterns{}, errors.New(err)
		}

		out.ancestor = ancestor
	}

	return out, nil
}

// compileExcludePattern compiles user `exclude_from_copy` patterns into one
// combined matcher. Each pattern is wrapped as `{<p>,<p>/**}` so excluding a
// directory excludes everything under it. Returns nil when patterns is
// empty.
func compileExcludePattern(patterns []string) (glob.Matcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	// Each pattern contributes two alternatives: itself and itself/**.
	const altsPerPattern = 2

	parts := make([]string, 0, altsPerPattern*len(patterns))

	for _, p := range patterns {
		normalized := strings.TrimRight(filepath.ToSlash(p), "/")
		if normalized == "" {
			continue
		}

		parts = append(parts, normalized, normalized+"/**")
	}

	if len(parts) == 0 {
		return nil, nil
	}

	matcher, err := glob.Compile("{" + strings.Join(parts, ",") + "}")
	if err != nil {
		return nil, errors.New(err)
	}

	return matcher, nil
}

// CopyFolderContentsWithFilter copies the files and folders within the source folder into the destination folder.
func CopyFolderContentsWithFilter(l log.Logger, source, destination, manifestFile string, filter func(absolutePath string) bool) error {
	const ownerReadWriteExecutePerms = 0o700
	if err := os.MkdirAll(destination, ownerReadWriteExecutePerms); err != nil {
		return errors.New(err)
	}

	manifest := NewFileManifest(destination, manifestFile)
	if err := manifest.Clean(l); err != nil {
		return errors.New(err)
	}

	if err := manifest.Create(); err != nil {
		return errors.New(err)
	}

	defer func(manifest *fileManifest) {
		err := manifest.Close()
		if err != nil {
			l.Warnf("Error closing manifest file: %v", err)
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
		fileRelativePath, err := filepath.Rel(source, file)
		if err != nil {
			return fmt.Errorf("relativize %q against source %q: %w", file, source, err)
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

			if err := CopyFolderContentsWithFilter(l, file, dest, manifestFile, filter); err != nil {
				return err
			}

			if err := manifest.AddDirectory(dest); err != nil {
				return err
			}
		} else {
			parentDir := filepath.Dir(dest)

			const ownerReadWriteExecutePerms = 0o700
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

// CopyFolderToTemp creates a temp directory with the given prefix, copies the
// contents of the source folder into it using the provided filter, and returns
// the path to the temp directory.
func CopyFolderToTemp(source string, tempPrefix string, filter func(path string) bool) (string, error) {
	dest, err := os.MkdirTemp("", tempPrefix)
	if err != nil {
		return "", errors.New(err)
	}

	if err := CopyFolderContentsWithFilter(log.New(), source, dest, ".copymanifest", filter); err != nil {
		return "", err
	}

	return dest, nil
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

	pathParts := strings.SplitSeq(path, string(filepath.Separator))
	for pathPart := range pathParts {
		if strings.HasPrefix(pathPart, ".") && pathPart != "." && pathPart != ".." {
			return true
		}
	}

	return false
}

// CopyFile copies a file from source to destination.
func CopyFile(source string, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return errors.New(err)
	}

	err = WriteFileWithSamePermissions(source, destination, file)

	return errors.New(errors.Join(err, file.Close()))
}

// WriteFileWithSamePermissions writes a file to the given destination with the given contents
// using the same permissions as the file at source.
func WriteFileWithSamePermissions(source string, destination string, contents io.Reader) error {
	fileInfo, err := os.Stat(source)
	if err != nil {
		return errors.New(err)
	}

	// CAS may place read-only files at the destination, which would block a plain open.
	if err := os.Remove(destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errors.New(err)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileInfo.Mode())
	if err != nil {
		return err
	}

	_, err = io.Copy(file, contents)

	return errors.Join(err, file.Close())
}

// ContainsPath returns true if path contains the given subpath
// E.g. path="foo/bar/bee", subpath="bar/bee" -> true
// E.g. path="foo/bar/bee", subpath="bar/be" -> false (because be is not a directory)
func ContainsPath(path, subpath string) bool {
	splitPath := strings.Split(filepath.Clean(path), string(filepath.Separator))
	splitSubpath := strings.Split(filepath.Clean(subpath), string(filepath.Separator))

	return ListContainsSublist(splitPath, splitSubpath)
}

// HasPathPrefix returns true if path starts with the given path prefix
// E.g. path="/foo/bar/biz", prefix="/foo/bar" -> true
// E.g. path="/foo/bar/biz", prefix="/foo/ba" -> false (because ba is not a directory
// path)
func HasPathPrefix(path, prefix string) bool {
	splitPath := strings.Split(filepath.Clean(path), string(filepath.Separator))
	splitPrefix := strings.Split(filepath.Clean(prefix), string(filepath.Separator))

	return ListHasPrefix(splitPath, splitPrefix)
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
	encoder        *gob.Encoder
	fileHandle     *os.File
	ManifestFolder string
	ManifestFile   string
}

// fileManifestEntry represents an entry in the fileManifest.
// It uses a struct with IsDir flag so that we won't have to call Stat on every
// file to determine if it's a directory or a file
type fileManifestEntry struct {
	Path  string
	IsDir bool
}

const (
	maxFileManifestEntries = 1_000_000
	maxFileManifests       = 100_000
)

// Clean removes files recorded in the manifest, keeping all operations bounded to ManifestFolder.
func (manifest *fileManifest) Clean(l log.Logger) error {
	rootDir, err := filepath.Abs(manifest.ManifestFolder)
	if err != nil {
		return errors.New(err)
	}

	manifestRelPath, ok := cleanRootRelPath(manifest.ManifestFile)
	if !ok {
		return errors.Errorf("manifest path %q must stay inside %q", manifest.ManifestFile, rootDir)
	}

	return manifest.clean(l, vfs.NewOSFS(), filepath.Clean(rootDir), manifestRelPath)
}

// clean reads manifests and removes their entries using root-confined vfs operations.
func (manifest *fileManifest) clean(l log.Logger, fsys vfs.FS, rootDir, manifestRelPath string) error {
	pending := []string{manifestRelPath}
	seen := make(map[string]struct{})

	for len(pending) > 0 {
		if len(seen) >= maxFileManifests {
			return errors.Errorf("manifest cleanup exceeded %d manifests", maxFileManifests)
		}

		last := len(pending) - 1
		currentRelPath := pending[last]
		pending = pending[:last]

		nextRelPaths, err := manifest.cleanOneManifest(l, fsys, rootDir, currentRelPath, seen)
		if err != nil {
			return err
		}

		pending = append(pending, nextRelPaths...)
	}

	return nil
}

func (manifest *fileManifest) cleanOneManifest(l log.Logger, fsys vfs.FS, rootDir, manifestRelPath string, seen map[string]struct{}) ([]string, error) {
	manifestPath := filepath.Join(rootDir, manifestRelPath)
	if _, visited := seen[manifestPath]; visited {
		l.Warnf("Skipping manifest %s: already processed", manifestPath)

		return nil, nil
	}

	file, ok, err := openManifestFileForClean(l, fsys, rootDir, manifestRelPath)
	if err != nil || !ok {
		return nil, err
	}

	seen[manifestPath] = struct{}{}

	defer closeAndRemoveManifest(l, file, fsys, rootDir, manifestRelPath)

	entries, err := decodeFileManifestEntries(gob.NewDecoder(file))
	if err != nil {
		l.Warnf("Ignoring invalid manifest %s: %v", manifestPath, err)

		return nil, nil
	}

	return manifest.cleanManifestEntries(l, fsys, rootDir, entries)
}

func openManifestFileForClean(l log.Logger, fsys vfs.FS, rootDir, manifestRelPath string) (vfs.File, bool, error) {
	parentHasSymlink, err := vfs.ParentPathHasSymlink(fsys, rootDir, manifestRelPath)
	if err != nil {
		return nil, false, err
	}

	if parentHasSymlink {
		l.Warnf("Skipping manifest %s: parent path contains a symlink", filepath.Join(rootDir, manifestRelPath))

		return nil, false, nil
	}

	manifestPath := filepath.Join(rootDir, manifestRelPath)

	info, err := vfs.Lstat(fsys, manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, removeManifestPath(fsys, rootDir, manifestRelPath)
	}

	file, err := fsys.Open(manifestPath)
	if err != nil {
		return nil, false, err
	}

	return file, true, nil
}

func closeAndRemoveManifest(l log.Logger, file vfs.File, fsys vfs.FS, rootDir, manifestRelPath string) {
	manifestPath := filepath.Join(rootDir, manifestRelPath)
	if err := file.Close(); err != nil {
		l.Warnf("Error closing file %s: %v", manifestPath, err)
	}

	if err := removeManifestPath(fsys, rootDir, manifestRelPath); err != nil {
		l.Warnf("Error removing manifest file %s: %v", manifestPath, err)
	}
}

func (manifest *fileManifest) cleanManifestEntries(l log.Logger, fsys vfs.FS, rootDir string, entries []fileManifestEntry) ([]string, error) {
	var manifestRelPaths []string

	for _, entry := range entries {
		manifestRelPath, err := manifest.cleanManifestEntry(l, fsys, rootDir, entry)
		if err != nil {
			return nil, err
		}

		if manifestRelPath != "" {
			manifestRelPaths = append(manifestRelPaths, manifestRelPath)
		}
	}

	return manifestRelPaths, nil
}

func (manifest *fileManifest) cleanManifestEntry(l log.Logger, fsys vfs.FS, rootDir string, entry fileManifestEntry) (string, error) {
	rel, ok := relPathInsideRoot(rootDir, entry.Path)
	if !ok {
		l.Warnf("Skipping manifest entry %q: resolves outside manifest root %q", entry.Path, rootDir)

		return "", nil
	}

	if entry.IsDir {
		return filepath.Join(rel, manifest.ManifestFile), nil
	}

	if err := removeManifestEntry(l, fsys, rootDir, rel); err != nil {
		return "", errors.New(err)
	}

	return "", nil
}

// Create will create the manifest file.
func (manifest *fileManifest) Create() error {
	const ownerWriteGlobalReadPerms = 0o644

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

func NewFileManifest(manifestFolder string, manifestFile string) *fileManifest {
	return &fileManifest{ManifestFolder: manifestFolder, ManifestFile: manifestFile}
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

	walkFunc := filepath.WalkDir
	if walkWithSymlinks {
		walkFunc = WalkDirWithSymlinks
	}

	err := walkFunc(directoryPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && IsTFFile(path) {
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

// EnsureCacheDir returns the global terragrunt cache directory for the current user.
func EnsureCacheDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", errors.New(err)
	}

	cacheDir = filepath.Join(cacheDir, "terragrunt")

	if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		return "", errors.New(err)
	}

	return cacheDir, nil
}

// EnsureTempDir returns the global terragrunt temp directory.
func EnsureTempDir() (string, error) {
	tempDir := filepath.Join(os.TempDir(), "terragrunt")

	if err := os.MkdirAll(tempDir, os.ModePerm); err != nil {
		return "", errors.New(err)
	}

	return tempDir, nil
}

// ExcludeFiltersFromFile returns a list of filters from the given filename, where each filter starts on a new line.
//
// Note that this is a backwards compatibility implementation for the `--queue-excludes-file` flag, so it's going to
// append the ! prefix to each filter to negate it.
func ExcludeFiltersFromFile(baseDir, filename string) ([]string, error) {
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

	var (
		lines   = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
		filters = make([]string, 0, len(lines))
	)

	for _, dir := range lines {
		dir = strings.TrimSpace(dir)
		if dir == "" || strings.HasPrefix(dir, "#") {
			continue
		}

		filters = append(filters, "!"+dir)
	}

	return filters, nil
}

// GetFiltersFromFile returns a list of filter queries from the given filename, where each filter query starts on a new line.
func GetFiltersFromFile(baseDir, filename string) ([]string, error) {
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

	var (
		lines   = strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
		filters = make([]string, 0, len(lines))
	)

	for _, filter := range lines {
		filter = strings.TrimSpace(filter)
		if filter == "" || strings.HasPrefix(filter, "#") {
			continue
		}

		filters = append(filters, filter)
	}

	return filters, nil
}

// MatchSha256Checksum returns the SHA256 checksum for the given file and filename.
func MatchSha256Checksum(file, filename []byte) []byte {
	var checksum []byte

	for line := range bytes.SplitSeq(file, []byte("\n")) {
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

// evalRealPathForWalkDir evaluates symlinks and returns the real path and whether it's a directory.
func evalRealPathForWalkDir(currentPath string) (string, bool, error) {
	realPath, err := filepath.EvalSymlinks(currentPath)
	if err != nil {
		return "", false, errors.Errorf("failed to evaluate symlinks for %s: %w", currentPath, err)
	}

	realInfo, err := os.Stat(realPath)
	if err != nil {
		return "", false, errors.Errorf("failed to describe file %s: %w", realPath, err)
	}

	return realPath, realInfo.IsDir(), nil
}

// WalkDirWithSymlinks traverses a directory tree using filepath.WalkDir, following symbolic links
// and calling the provided function for each file or directory encountered. It handles both regular
// symlinks and circular symlinks without getting into infinite loops.
//
//nolint:funlen
func WalkDirWithSymlinks(root string, externalWalkFn fs.WalkDirFunc) error {
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
		return filepath.WalkDir(pair.physical, func(currentPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return externalWalkFn(currentPath, d, err)
			}

			// Convert the current physical path to a logical path relative to the walk root
			rel, err := filepath.Rel(pair.physical, currentPath)
			if err != nil {
				return errors.Errorf("failed to get relative path between %s and %s: %w", pair.physical, currentPath, err)
			}

			logicalPath := filepath.Join(pair.logical, rel)

			// Call the provided function only if we haven't seen this logical path before
			if !visitedLogical[logicalPath] {
				visitedLogical[logicalPath] = true

				if err := externalWalkFn(logicalPath, d, nil); err != nil {
					return err
				}
			}

			// If we encounter a symlink, resolve and follow it
			if d.Type()&fs.ModeSymlink != 0 {
				realPath, isDir, evalErr := evalRealPathForWalkDir(currentPath)
				if evalErr != nil {
					return evalErr
				}

				// Skip if we've seen this symlink->target combination before
				// This prevents infinite loops with circular symlinks
				if visited[realPath+":"+currentPath] {
					return nil
				}

				visited[realPath+":"+currentPath] = true

				// If the target is a directory, recursively walk it
				if isDir {
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
		return errors.Errorf("failed to evaluate symlinks for %s: %w", root, err)
	}

	// Start the walk from the root directory
	return walkFn(pathPair{
		physical: realRoot,
		logical:  realRoot,
	})
}

// SanitizePath resolves a file path within a base directory, returning the sanitized path or an error if it attempts
// to access anything outside the base directory.
func SanitizePath(baseDir string, file string) (sanitized string, err error) {
	if baseDir == "" || file == "" {
		return "", errors.New("baseDir and file must be provided")
	}

	file, err = url.QueryUnescape(file)
	if err != nil {
		return "", err
	}

	baseDir, err = url.QueryUnescape(baseDir)
	if err != nil {
		return "", err
	}

	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return "", err
	}

	defer func() {
		if cerr := root.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := root.Stat(file); err != nil {
		return "", err
	}

	// Preserve nested directories from the validated input. Using
	// fileInfo.Name() would flatten "a/b/c.txt" to "<baseDir>/c.txt".
	// root.Stat already rejects paths that escape baseDir, so we only need
	// to clean the input and join it back onto baseDir.
	cleanedRelative := filepath.Clean(file)
	cleanedRelative = strings.TrimLeft(cleanedRelative, string(os.PathSeparator))

	return filepath.Join(baseDir, cleanedRelative), nil
}

// RelPathForLog returns a relative path suitable for logging.
// If the path cannot be made relative, it returns the original path.
// Paths that don't start with ".." get a "./" prefix for clarity.
// If showAbsPath is true, the original targetPath is returned unchanged.
func RelPathForLog(basePath, targetPath string, showAbsPath bool) string {
	if showAbsPath {
		return targetPath
	}

	if relPath, err := filepath.Rel(basePath, targetPath); err == nil {
		if relPath == "." {
			return targetPath
		}

		// Add "./" prefix for paths within the base directory for clarity
		if !strings.HasPrefix(relPath, "..") {
			return "." + string(filepath.Separator) + relPath
		}

		return relPath
	}

	return targetPath
}

// ResolvePath resolves symlinks in a path for consistent comparison across platforms.
// On macOS, /var is a symlink to /private/var, so paths must be resolved.
// Returns the original path if symlink resolution fails.
func ResolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}

	return resolved
}

// MoveFile attempts to rename a file from source to destination, if this fails
// due to invalid cross-device link it falls back to copying the file contents
// and deleting the original file.
func MoveFile(source string, destination string) error {
	if renameErr := os.Rename(source, destination); renameErr != nil {
		var sysErr syscall.Errno
		if errors.As(renameErr, &sysErr) && sysErr == syscall.EXDEV {
			if moveErr := CopyFile(source, destination); moveErr != nil {
				return moveErr
			}

			return os.Remove(source)
		}

		return renameErr
	}

	return nil
}

// SkipDirIfIgnorable checks if an entire directory should be skipped based on the fact that it's
// in a directory that should never have components discovered in it.
func SkipDirIfIgnorable(dir string) error {
	switch dir {
	case GitDir, TerraformCacheDir, TerragruntCacheDir:
		return filepath.SkipDir
	}

	return nil
}

func decodeFileManifestEntries(decoder *gob.Decoder) ([]fileManifestEntry, error) {
	var entries []fileManifestEntry

	for {
		var entry fileManifestEntry
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				return entries, nil
			}

			return nil, err
		}

		entries = append(entries, entry)
		if len(entries) > maxFileManifestEntries {
			return nil, errors.Errorf("manifest contains more than %d entries", maxFileManifestEntries)
		}
	}
}

func removeManifestEntry(l log.Logger, fsys vfs.FS, rootDir, rel string) error {
	rel, ok := cleanRootRelPath(rel)
	if !ok {
		return nil
	}

	hasSymlink, err := vfs.ParentPathHasSymlink(fsys, rootDir, rel)
	if err != nil {
		return err
	}

	if hasSymlink {
		l.Warnf("Skipping manifest entry %s: parent path contains a symlink", filepath.Join(rootDir, rel))

		return nil
	}

	if err := fsys.Remove(filepath.Join(rootDir, rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func removeManifestPath(fsys vfs.FS, rootDir, rel string) error {
	rel, ok := cleanRootRelPath(rel)
	if !ok {
		return nil
	}

	hasSymlink, err := vfs.ParentPathHasSymlink(fsys, rootDir, rel)
	if err != nil {
		return err
	}

	if hasSymlink {
		return nil
	}

	if err := fsys.RemoveAll(filepath.Join(rootDir, rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func cleanRootRelPath(rel string) (string, bool) {
	rel = filepath.Clean(rel)
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return rel, true
}

// relPathInsideRoot returns target relative to rootDir; ok=false when target escapes.
// Relative manifest entries are resolved against the process CWD, not rootDir,
// because adversarial relative paths must be judged the same way os.Remove used
// to resolve legacy entries. Do not replace this with filepath.Join(rootDir, target).
func relPathInsideRoot(rootDir, target string) (string, bool) {
	if !filepath.IsAbs(rootDir) {
		return "", false
	}

	cleanTarget := filepath.Clean(target)
	if !filepath.IsAbs(cleanTarget) {
		absTarget, err := filepath.Abs(cleanTarget)
		if err != nil {
			return "", false
		}

		cleanTarget = absTarget
	}

	rel, err := filepath.Rel(filepath.Clean(rootDir), cleanTarget)
	if err != nil {
		return "", false
	}

	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return cleanRootRelPath(rel)
}
