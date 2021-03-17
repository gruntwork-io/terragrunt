package tfsource

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"
	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

var forcedRegexp = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)

// This struct represents information about Terraform source code that needs to be downloaded
type TerraformSource struct {
	// A canonical version of RawSource, in URL format
	CanonicalSourceURL *url.URL

	// The folder where we should download the source to
	DownloadDir string

	// The folder in DownloadDir that should be used as the working directory for Terraform
	WorkingDir string

	// The path to a file in DownloadDir that stores the version number of the code
	VersionFile string
}

func (src *TerraformSource) String() string {
	return fmt.Sprintf("TerraformSource{CanonicalSourceURL = %v, DownloadDir = %v, WorkingDir = %v, VersionFile = %v}", src.CanonicalSourceURL, src.DownloadDir, src.WorkingDir, src.VersionFile)
}

// Encode a version number for the given source. When calculating a version number, we take the query
// string of the source URL, calculate its sha1, and base 64 encode it. For remote URLs (e.g. Git URLs), this is
// based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module
// name and the query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string,
// so the same file path (/foo/bar) is always considered the same version. See also the encodeSourceName and
// ProcessTerraformSource methods.
func (terraformSource TerraformSource) EncodeSourceVersion() string {
	return util.EncodeBase64Sha1(terraformSource.CanonicalSourceURL.Query().Encode())
}

// Write a file into the DownloadDir that contains the version number of this source code. The version number is
// calculated using the EncodeSourceVersion method.
func (terraformSource TerraformSource) WriteVersionFile() error {
	version := terraformSource.EncodeSourceVersion()
	return errors.WithStackTrace(ioutil.WriteFile(terraformSource.VersionFile, []byte(version), 0640))
}

// Take the given source path and create a TerraformSource struct from it, including the folder where the source should
// be downloaded to. Our goal is to reuse the download folder for the same source URL between Terragrunt runs.
// Otherwise, for every Terragrunt command, you'd have to wait for Terragrunt to download your Terraform code, download
// that code's dependencies (terraform get), and configure remote state (terraform remote config), which is very slow.
//
// To maximize reuse, given a working directory w and a source URL s, we download code from S into the folder /T/W/H
// where:
//
// 1. S is the part of s before the double-slash (//). This typically represents the root of the repo (e.g.
//    github.com/foo/infrastructure-modules). We download the entire repo so that relative paths to other files in that
//    repo resolve correctly. If no double-slash is specified, all of s is used.
// 1. T is the OS temp dir (e.g. /tmp).
// 2. W is the base 64 encoded sha1 hash of w. This ensures that if you are running Terragrunt concurrently in
//    multiple folders (e.g. during automated tests), then even if those folders are using the same source URL s, they
//    do not overwrite each other.
// 3. H is the base 64 encoded sha1 of S without its query string. For remote source URLs (e.g. Git
//    URLs), this is based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar)
//    identifies the repo, and we always want to download the same repo into the same folder (see the encodeSourceName
//    method). We also assume the version of the module is stored in the query string (e.g. ref=v0.0.3), so we store
//    the base 64 encoded sha1 of the query string in a file called .terragrunt-source-version within /T/W/H.
//
// The downloadTerraformSourceIfNecessary decides when we should download the Terraform code and when not to. It uses
// the following rules:
//
// 1. Always download source URLs pointing to local file paths.
// 2. Only download source URLs pointing to remote paths if /T/W/H doesn't already exist or, if it does exist, if the
//    version number in /T/W/H/.terragrunt-source-version doesn't match the current version.
func NewTerraformSource(source string, downloadDir string, workingDir string, logger *logrus.Entry) (*TerraformSource, error) {

	canonicalWorkingDir, err := util.CanonicalPath(workingDir, "")
	if err != nil {
		return nil, err
	}

	canonicalSourceUrl, err := toSourceUrl(source, canonicalWorkingDir)
	if err != nil {
		return nil, err
	}

	rootSourceUrl, modulePath, err := splitSourceUrl(canonicalSourceUrl, logger)
	if err != nil {
		return nil, err
	}

	if IsLocalSource(rootSourceUrl) {
		// Always use canonical file paths for local source folders, rather than relative paths, to ensure
		// that the same local folder always maps to the same download folder, no matter how the local folder
		// path is specified
		canonicalFilePath, err := util.CanonicalPath(rootSourceUrl.Path, "")
		if err != nil {
			return nil, err
		}
		rootSourceUrl.Path = canonicalFilePath
	}

	rootPath, err := encodeSourceName(rootSourceUrl)
	if err != nil {
		return nil, err
	}

	encodedWorkingDir := util.EncodeBase64Sha1(canonicalWorkingDir)
	updatedDownloadDir := util.JoinPath(downloadDir, encodedWorkingDir, rootPath)
	updatedWorkingDir := util.JoinPath(updatedDownloadDir, modulePath)
	versionFile := util.JoinPath(updatedDownloadDir, ".terragrunt-source-version")

	return &TerraformSource{
		CanonicalSourceURL: rootSourceUrl,
		DownloadDir:        updatedDownloadDir,
		WorkingDir:         updatedWorkingDir,
		VersionFile:        versionFile,
	}, nil
}

// Convert the given source into a URL struct. This method should be able to handle all source URLs that the terraform
// init command can handle, parsing local file paths, Git paths, and HTTP URLs correctly.
func toSourceUrl(source string, workingDir string) (*url.URL, error) {
	// The go-getter library is what Terraform's init command uses to download source URLs. Use that library to
	// parse the URL.
	rawSourceUrlWithGetter, err := getter.Detect(source, workingDir, getter.Detectors)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return parseSourceUrl(rawSourceUrlWithGetter)
}

// Parse the given source URL into a URL struct. This method can handle source URLs that include go-getter's "forced
// getter" prefixes, such as git::.
func parseSourceUrl(source string) (*url.URL, error) {
	forcedGetters := []string{}
	// Continuously strip the forced getters until there is no more. This is to handle complex URL schemes like the
	// git-remote-codecommit style URL.
	forcedGetter, rawSourceUrl := getForcedGetter(source)
	for forcedGetter != "" {
		// Prepend like a stack, so that we prepend to the URL scheme in the right order.
		forcedGetters = append([]string{forcedGetter}, forcedGetters...)
		forcedGetter, rawSourceUrl = getForcedGetter(rawSourceUrl)
	}

	// Parse the URL without the getter prefix
	canonicalSourceUrl, err := urlhelper.Parse(rawSourceUrl)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// Reattach the "getter" prefix as part of the scheme
	for _, forcedGetter := range forcedGetters {
		canonicalSourceUrl.Scheme = fmt.Sprintf("%s::%s", forcedGetter, canonicalSourceUrl.Scheme)
	}

	return canonicalSourceUrl, nil
}

// Returns true if the given URL refers to a path on the local file system
func IsLocalSource(sourceUrl *url.URL) bool {
	return sourceUrl.Scheme == "file"
}

// Splits a source URL into the root repo and the path. The root repo is the part of the URL before the double-slash
// (//), which typically represents the root of a modules repo (e.g. github.com/foo/infrastructure-modules) and the
// path is everything after the double slash. If there is no double-slash in the URL, the root repo is the entire
// sourceUrl and the path is an empty string.
func splitSourceUrl(sourceUrl *url.URL, logger *logrus.Entry) (*url.URL, string, error) {
	pathSplitOnDoubleSlash := strings.SplitN(sourceUrl.Path, "//", 2)

	if len(pathSplitOnDoubleSlash) > 1 {
		sourceUrlModifiedPath, err := parseSourceUrl(sourceUrl.String())
		if err != nil {
			return nil, "", errors.WithStackTrace(err)
		}

		sourceUrlModifiedPath.Path = pathSplitOnDoubleSlash[0]
		return sourceUrlModifiedPath, pathSplitOnDoubleSlash[1], nil
	} else {
		logger.Warningf("No double-slash (//) found in source URL %s. Relative paths in downloaded Terraform code may not work.", sourceUrl.Path)
		return sourceUrl, "", nil
	}
}

// Encode a the module name for the given source URL. When calculating a module name, we calculate the base 64 encoded
// sha1 of the entire source URL without the query string. For remote URLs (e.g. Git URLs), this is based on the
// assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module name and the
// query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string, so the same
// file path (/foo/bar) is always considered the same version. See also the EncodeSourceVersion and
// ProcessTerraformSource methods.
func encodeSourceName(sourceUrl *url.URL) (string, error) {
	sourceUrlNoQuery, err := parseSourceUrl(sourceUrl.String())
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	sourceUrlNoQuery.RawQuery = ""

	return util.EncodeBase64Sha1(sourceUrlNoQuery.String()), nil
}

// Terraform source URLs can contain a "getter" prefix that specifies the type of protocol to use to download that URL,
// such as "git::", which means Git should be used to download the URL. This method returns the getter prefix and the
// rest of the URL. This code is copied from the getForcedGetter method of go-getter/get.go, as that method is not
// exported publicly.
func getForcedGetter(sourceUrl string) (string, string) {
	if matches := forcedRegexp.FindStringSubmatch(sourceUrl); matches != nil && len(matches) > 2 {
		return matches[1], matches[2]
	}

	return "", sourceUrl
}
