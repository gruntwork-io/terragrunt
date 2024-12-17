package terraform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

var (
	forcedRegexp     = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)
	httpSchemeRegexp = regexp.MustCompile(`(?i)^https?://`)
)

const matchCount = 2

// Source represents information about Terraform source code that needs to be downloaded.
type Source struct {
	// A canonical version of RawSource, in URL format
	CanonicalSourceURL *url.URL

	// The folder where we should download the source to
	DownloadDir string

	// The folder in DownloadDir that should be used as the working directory for Terraform
	WorkingDir string

	// The path to a file in DownloadDir that stores the version number of the code
	VersionFile string

	// Logger to use for logging
	Logger log.Logger

	// WalkWithSymlinks controls whether to walk symlinks in the downloaded source
	WalkWithSymlinks bool
}

func (src Source) String() string {
	return fmt.Sprintf("Source{CanonicalSourceURL = %v, DownloadDir = %v, WorkingDir = %v, VersionFile = %v}", src.CanonicalSourceURL, src.DownloadDir, src.WorkingDir, src.VersionFile)
}

// EncodeSourceVersion encodes a version number for the given source. When calculating a version number, we take the query
// string of the source URL, calculate its sha1, and base 64 encode it. For remote URLs (e.g. Git URLs), this is
// based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module
// name and the query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string,
// so the same file path (/foo/bar) is always considered the same version. To detect changes the file path will be hashed
// and returned as version. In case of hash error the default encoded source version will be returned.
// See also the encodeSourceName and ProcessTerraformSource methods.
func (src Source) EncodeSourceVersion() (string, error) {
	if IsLocalSource(src.CanonicalSourceURL) {
		sourceHash := sha256.New()
		sourceDir := filepath.Clean(src.CanonicalSourceURL.Path)

		var err error
		if src.WalkWithSymlinks {
			err = util.WalkWithSymlinks(sourceDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					// If we've encountered an error while walking the tree, give up
					return err
				}

				if info.IsDir() {
					// We don't use any info from directories to calculate our hash
					return nil
				}
				// avoid checking files in .terragrunt-cache directory since contents is auto-generated
				if strings.Contains(path, util.TerragruntCacheDir) {
					return nil
				}
				// avoid checking files in .terraform directory since contents is auto-generated
				if info.Name() == util.TerraformLockFile {
					return nil
				}

				fileModified := info.ModTime().UnixMicro()
				hashContents := fmt.Sprintf("%s:%d", path, fileModified)
				sourceHash.Write([]byte(hashContents))

				return nil
			})
		} else {
			err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					// If we've encountered an error while walking the tree, give up
					return err
				}

				if info.IsDir() {
					// We don't use any info from directories to calculate our hash
					return nil
				}
				// avoid checking files in .terragrunt-cache directory since contents is auto-generated
				if strings.Contains(path, util.TerragruntCacheDir) {
					return nil
				}
				// avoid checking files in .terraform directory since contents is auto-generated
				if info.Name() == util.TerraformLockFile {
					return nil
				}

				fileModified := info.ModTime().UnixMicro()
				hashContents := fmt.Sprintf("%s:%d", path, fileModified)
				sourceHash.Write([]byte(hashContents))

				return nil
			})
		}

		if err == nil {
			hash := hex.EncodeToString(sourceHash.Sum(nil))

			return hash, nil
		}

		src.Logger.WithError(err).Warnf("Could not encode version for local source")

		return "", err
	}

	return util.EncodeBase64Sha1(src.CanonicalSourceURL.Query().Encode()), nil
}

// WriteVersionFile writes a file into the DownloadDir that contains
// the version number of this source code. The version number is
// calculated using the EncodeSourceVersion method.
func (src Source) WriteVersionFile() error {
	version, err := src.EncodeSourceVersion()
	if err != nil {
		// If we failed to calculate a SHA of the downloaded source, write a SHA of
		// some random data into the version file.
		//
		// This ensures we attempt to redownload the source next time.
		version, err = util.GenerateRandomSha256()
		if err != nil {
			return errors.New(err)
		}
	}

	const ownerReadWriteGroupReadPerms = 0640

	return errors.New(os.WriteFile(src.VersionFile, []byte(version), ownerReadWriteGroupReadPerms))
}

// NewSource takes the given source path and create a Source struct from it, including the folder where the source should
// be downloaded to. Our goal is to reuse the download folder for the same source URL between Terragrunt runs.
// Otherwise, for every Terragrunt command, you'd have to wait for Terragrunt to download your Terraform code, download
// that code's dependencies (terraform get), and configure remote state (terraform remote config), which is very slow.
//
// To maximize reuse, given a working directory w and a source URL s, we download code from S into the folder /T/W/H
// where:
//
//  1. S is the part of s before the double-slash (//). This typically represents the root of the repo (e.g.
//     github.com/foo/infrastructure-modules). We download the entire repo so that relative paths to other files in that
//     repo resolve correctly. If no double-slash is specified, all of s is used.
//  1. T is the OS temp dir (e.g. /tmp).
//  2. W is the base 64 encoded sha1 hash of w. This ensures that if you are running Terragrunt concurrently in
//     multiple folders (e.g. during automated tests), then even if those folders are using the same source URL s, they
//     do not overwrite each other.
//  3. H is the base 64 encoded sha1 of S without its query string. For remote source URLs (e.g. Git
//     URLs), this is based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar)
//     identifies the repo, and we always want to download the same repo into the same folder (see the encodeSourceName
//     method). We also assume the version of the module is stored in the query string (e.g. ref=v0.0.3), so we store
//     the base 64 encoded sha1 of the query string in a file called .terragrunt-source-version within /T/W/H.
//
// The downloadTerraformSourceIfNecessary decides when we should download the Terraform code and when not to. It uses
// the following rules:
//
//  1. Always download source URLs pointing to local file paths.
//  2. Only download source URLs pointing to remote paths if /T/W/H doesn't already exist or, if it does exist, if the
//     version number in /T/W/H/.terragrunt-source-version doesn't match the current version.
func NewSource(source string, downloadDir string, workingDir string, logger log.Logger, walkWithSymlinks bool) (*Source, error) {
	canonicalWorkingDir, err := util.CanonicalPath(workingDir, "")
	if err != nil {
		return nil, err
	}

	canonicalSourceURL, err := ToSourceURL(source, canonicalWorkingDir)
	if err != nil {
		return nil, err
	}

	rootSourceURL, modulePath, err := SplitSourceURL(canonicalSourceURL, logger)
	if err != nil {
		return nil, err
	}

	if IsLocalSource(rootSourceURL) {
		// Always use canonical file paths for local source folders, rather than relative paths, to ensure
		// that the same local folder always maps to the same download folder, no matter how the local folder
		// path is specified
		canonicalFilePath, err := util.CanonicalPath(rootSourceURL.Path, "")
		if err != nil {
			return nil, err
		}

		rootSourceURL.Path = canonicalFilePath
	}

	rootPath, err := encodeSourceName(rootSourceURL)
	if err != nil {
		return nil, err
	}

	encodedWorkingDir := util.EncodeBase64Sha1(canonicalWorkingDir)
	updatedDownloadDir := util.JoinPath(downloadDir, encodedWorkingDir, rootPath)
	updatedWorkingDir := util.JoinPath(updatedDownloadDir, modulePath)
	versionFile := util.JoinPath(updatedDownloadDir, ".terragrunt-source-version")

	return &Source{
		CanonicalSourceURL: rootSourceURL,
		DownloadDir:        updatedDownloadDir,
		WorkingDir:         updatedWorkingDir,
		VersionFile:        versionFile,
		Logger:             logger,
		WalkWithSymlinks:   walkWithSymlinks,
	}, nil
}

// ToSourceURL converts the given source into a URL struct.
// This method should be able to handle all source URLs that the terraform
// init command can handle, parsing local file paths, Git paths, and HTTP URLs correctly.
func ToSourceURL(source string, workingDir string) (*url.URL, error) {
	source, err := normalizeSourceURL(source, workingDir)
	if err != nil {
		return nil, err
	}

	// The go-getter library is what Terraform's init command uses to download source URLs. Use that library to
	// parse the URL.
	rawSourceURLWithGetter, err := getter.Detect(source, workingDir, getter.Detectors)
	if err != nil {
		return nil, errors.New(err)
	}

	return parseSourceURL(rawSourceURLWithGetter)
}

// We have to remove the http(s) scheme from the source URL to allow `getter.Detect` to add the source type, but only if the `getter` has a detector for that host.
func normalizeSourceURL(source string, workingDir string) (string, error) {
	newSource := httpSchemeRegexp.ReplaceAllString(source, "")

	// We can't use `the getter.Detectors` global variable because we need to exclude from checking:
	// * `getter.FileDetector` is not a host detector
	// * `getter.S3Detector` we should not remove `https` from s3 link since this is a public link, and if we remove `https` scheme, `getter.S3Detector` adds `s3::https` which in turn requires credentials.
	detectors := []getter.Detector{
		new(getter.GitHubDetector),
		new(getter.GitLabDetector),
		new(getter.GitDetector),
		new(getter.BitBucketDetector),
		new(getter.GCSDetector),
	}

	for _, detector := range detectors {
		_, ok, err := detector.Detect(newSource, workingDir)
		if err != nil {
			return source, errors.New(err)
		}

		if ok {
			return newSource, nil
		}
	}

	return source, nil
}

// Parse the given source URL into a URL struct. This method can handle source URLs that include go-getter's "forced
// getter" prefixes, such as git::.
func parseSourceURL(source string) (*url.URL, error) {
	forcedGetters := []string{}
	// Continuously strip the forced getters until there is no more. This is to handle complex URL schemes like the
	// git-remote-codecommit style URL.
	forcedGetter, rawSourceURL := getForcedGetter(source)
	for forcedGetter != "" {
		// Prepend like a stack, so that we prepend to the URL scheme in the right order.
		forcedGetters = append([]string{forcedGetter}, forcedGetters...)
		forcedGetter, rawSourceURL = getForcedGetter(rawSourceURL)
	}

	// Parse the URL without the getter prefix
	canonicalSourceURL, err := urlhelper.Parse(rawSourceURL)
	if err != nil {
		return nil, errors.New(err)
	}

	// Reattach the "getter" prefix as part of the scheme
	for _, forcedGetter := range forcedGetters {
		canonicalSourceURL.Scheme = fmt.Sprintf("%s::%s", forcedGetter, canonicalSourceURL.Scheme)
	}

	return canonicalSourceURL, nil
}

// IsLocalSource returns true if the given URL refers to a path on the local file system
func IsLocalSource(sourceURL *url.URL) bool {
	return sourceURL.Scheme == "file"
}

// SplitSourceURL splits a source URL into the root repo and the path. The root repo is the part of the URL before the double-slash
// (//), which typically represents the root of a modules repo (e.g. github.com/foo/infrastructure-modules) and the
// path is everything after the double slash. If there is no double-slash in the URL, the root repo is the entire
// sourceUrl and the path is an empty string.
func SplitSourceURL(sourceURL *url.URL, logger log.Logger) (*url.URL, string, error) {
	pathSplitOnDoubleSlash := strings.SplitN(sourceURL.Path, "//", 2) //nolint:mnd

	if len(pathSplitOnDoubleSlash) > 1 {
		sourceURLModifiedPath, err := parseSourceURL(sourceURL.String())
		if err != nil {
			return nil, "", errors.New(err)
		}

		sourceURLModifiedPath.Path = pathSplitOnDoubleSlash[0]

		return sourceURLModifiedPath, pathSplitOnDoubleSlash[1], nil
	}
	// check if path is remote URL
	if sourceURL.Scheme != "" {
		return sourceURL, "", nil
	}
	// check if sourceUrl.Path is a local file path
	_, err := os.Stat(sourceURL.Path)
	if err != nil {
		// log warning message to notify user that sourceUrl.Path may not work
		logger.Warnf("No double-slash (//) found in source URL %s. Relative paths in downloaded Terraform code may not work.", sourceURL.Path)
	}

	return sourceURL, "", nil
}

// Encode a the module name for the given source URL. When calculating a module name, we calculate the base 64 encoded
// sha1 of the entire source URL without the query string. For remote URLs (e.g. Git URLs), this is based on the
// assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module name and the
// query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string, so the same
// file path (/foo/bar) is always considered the same version. See also the EncodeSourceVersion and
// ProcessTerraformSource methods.
func encodeSourceName(sourceURL *url.URL) (string, error) {
	sourceURLNoQuery, err := parseSourceURL(sourceURL.String())
	if err != nil {
		return "", errors.New(err)
	}

	sourceURLNoQuery.RawQuery = ""

	return util.EncodeBase64Sha1(sourceURLNoQuery.String()), nil
}

// Terraform source URLs can contain a "getter" prefix that specifies the type of protocol to use to download that URL,
// such as "git::", which means Git should be used to download the URL. This method returns the getter prefix and the
// rest of the URL. This code is copied from the getForcedGetter method of go-getter/get.go, as that method is not
// exported publicly.
func getForcedGetter(sourceURL string) (string, string) {
	if matches := forcedRegexp.FindStringSubmatch(sourceURL); len(matches) > matchCount {
		return matches[1], matches[2]
	}

	return "", sourceURL
}
