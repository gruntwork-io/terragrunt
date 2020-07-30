package cli

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"strings"

	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"
)

// manifest for files coped from terragrunt module folder (i.e., the folder that contains the current terragrunt.hcl)
const MODULE_MANIFEST_NAME = ".terragrunt-module-manifest"

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

var forcedRegexp = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 3. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the processTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformSource(source string, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terraformSource, err := processTerraformSource(source, terragruntOptions)
	if err != nil {
		return err
	}

	if err := downloadTerraformSourceIfNecessary(terraformSource, terragruntOptions, terragruntConfig); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Copying files from %s into %s", terragruntOptions.WorkingDir, terraformSource.WorkingDir)
	if err := util.CopyFolderContents(terragruntOptions.WorkingDir, terraformSource.WorkingDir, MODULE_MANIFEST_NAME); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Setting working directory to %s", terraformSource.WorkingDir)
	terragruntOptions.WorkingDir = terraformSource.WorkingDir

	return nil
}

// Download the specified TerraformSource if the latest code hasn't already been downloaded.
func downloadTerraformSourceIfNecessary(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	if terragruntOptions.SourceUpdate {
		terragruntOptions.Logger.Printf("The --%s flag is set, so deleting the temporary folder %s before downloading source.", OPT_TERRAGRUNT_SOURCE_UPDATE, terraformSource.DownloadDir)
		if err := os.RemoveAll(terraformSource.DownloadDir); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	alreadyLatest, err := alreadyHaveLatestCode(terraformSource, terragruntOptions)
	if err != nil {
		return err
	}

	if alreadyLatest {
		terragruntOptions.Logger.Printf("Terraform files in %s are up to date. Will not download again.", terraformSource.WorkingDir)
		return nil
	}

	// When downloading source, we need to process any hooks waiting on `init-from-module`. Therefore, we clone the
	// options struct, set the command to the value the hooks are expecting, and run the download action surrounded by
	// before and after hooks (if any).
	terragruntOptionsForDownload := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsForDownload.TerraformCommand = CMD_INIT_FROM_MODULE
	downloadErr := runActionWithHooks("download source", terragruntOptionsForDownload, terragruntConfig, func() error {
		return downloadSource(terraformSource, terragruntOptions, terragruntConfig)
	})

	if downloadErr != nil {
		return downloadErr
	}

	if err := writeVersionFile(terraformSource); err != nil {
		return err
	}

	return nil
}

// Returns true if the specified TerraformSource, of the exact same version, has already been downloaded into the
// DownloadFolder. This helps avoid downloading the same code multiple times. Note that if the TerraformSource points
// to a local file path, we assume the user is doing local development and always return false to ensure the latest
// code is downloaded (or rather, copied) every single time. See the processTerraformSource method for more info.
func alreadyHaveLatestCode(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if isLocalSource(terraformSource.CanonicalSourceURL) ||
		!util.FileExists(terraformSource.DownloadDir) ||
		!util.FileExists(terraformSource.WorkingDir) ||
		!util.FileExists(terraformSource.VersionFile) {

		return false, nil
	}

	tfFiles, err := filepath.Glob(fmt.Sprintf("%s/*.tf", terraformSource.WorkingDir))
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	if len(tfFiles) == 0 {
		terragruntOptions.Logger.Printf("Working dir %s exists but contains no Terraform files, so assuming code needs to be downloaded again.", terraformSource.WorkingDir)
		return false, nil
	}

	currentVersion := encodeSourceVersion(terraformSource.CanonicalSourceURL)
	previousVersion, err := readVersionFile(terraformSource)

	if err != nil {
		return false, err
	}

	return previousVersion == currentVersion, nil
}

// Return the version number stored in the DownloadDir. This version number can be used to check if the Terraform code
// that has already been downloaded is the same as the version the user is currently requesting. The version number is
// calculated using the encodeSourceVersion method.
func readVersionFile(terraformSource *TerraformSource) (string, error) {
	return util.ReadFileAsString(terraformSource.VersionFile)
}

// Write a file into the DownloadDir that contains the version number of this source code. The version number is
// calculated using the encodeSourceVersion method.
func writeVersionFile(terraformSource *TerraformSource) error {
	version := encodeSourceVersion(terraformSource.CanonicalSourceURL)
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
func processTerraformSource(source string, terragruntOptions *options.TerragruntOptions) (*TerraformSource, error) {
	canonicalWorkingDir, err := util.CanonicalPath(terragruntOptions.WorkingDir, "")
	if err != nil {
		return nil, err
	}

	canonicalSourceUrl, err := toSourceUrl(source, canonicalWorkingDir)
	if err != nil {
		return nil, err
	}

	rootSourceUrl, modulePath, err := splitSourceUrl(canonicalSourceUrl, terragruntOptions)
	if err != nil {
		return nil, err
	}

	if isLocalSource(rootSourceUrl) {
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
	downloadDir := util.JoinPath(terragruntOptions.DownloadDir, encodedWorkingDir, rootPath)
	workingDir := util.JoinPath(downloadDir, modulePath)
	versionFile := util.JoinPath(downloadDir, ".terragrunt-source-version")

	return &TerraformSource{
		CanonicalSourceURL: rootSourceUrl,
		DownloadDir:        downloadDir,
		WorkingDir:         workingDir,
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
	forcedGetter, rawSourceUrl := getForcedGetter(source)

	// Parse the URL without the getter prefix
	canonicalSourceUrl, err := urlhelper.Parse(rawSourceUrl)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	// Reattach the "getter" prefix as part of the scheme
	if forcedGetter != "" {
		canonicalSourceUrl.Scheme = fmt.Sprintf("%s::%s", forcedGetter, canonicalSourceUrl.Scheme)
	}

	return canonicalSourceUrl, nil
}

// Terraform source URLs can contain a "getter" prefix that specifies the type of protocol to use to download that URL,
// such as "git::", which means Git should be used to download the URL. This method returns the getter prefix and the
// rest of the URL. This code is copied from the getForcedGetter method of go-getter/get.go, as that method is not
// exported publicly.
func getForcedGetter(sourceUrl string) (string, string) {
	if matches := forcedRegexp.FindStringSubmatch(sourceUrl); len(matches) > 2 {
		return matches[1], matches[2]
	}

	return "", sourceUrl
}

// Splits a source URL into the root repo and the path. The root repo is the part of the URL before the double-slash
// (//), which typically represents the root of a modules repo (e.g. github.com/foo/infrastructure-modules) and the
// path is everything after the double slash. If there is no double-slash in the URL, the root repo is the entire
// sourceUrl and the path is an empty string.
func splitSourceUrl(sourceUrl *url.URL, terragruntOptions *options.TerragruntOptions) (*url.URL, string, error) {
	pathSplitOnDoubleSlash := strings.SplitN(sourceUrl.Path, "//", 2)

	if len(pathSplitOnDoubleSlash) > 1 {
		sourceUrlModifiedPath, err := parseSourceUrl(sourceUrl.String())
		if err != nil {
			return nil, "", errors.WithStackTrace(err)
		}

		sourceUrlModifiedPath.Path = pathSplitOnDoubleSlash[0]
		return sourceUrlModifiedPath, pathSplitOnDoubleSlash[1], nil
	} else {
		terragruntOptions.Logger.Printf("WARNING: no double-slash (//) found in source URL %s. Relative paths in downloaded Terraform code may not work.", sourceUrl.Path)
		return sourceUrl, "", nil
	}
}

// Encode a version number for the given source URL. When calculating a version number, we simply take the query
// string of the source URL, calculate its sha1, and base 64 encode it. For remote URLs (e.g. Git URLs), this is
// based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module
// name and the query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string,
// so the same file path (/foo/bar) is always considered the same version. See also the encodeSourceName and
// processTerraformSource methods.
func encodeSourceVersion(sourceUrl *url.URL) string {
	return util.EncodeBase64Sha1(sourceUrl.Query().Encode())
}

// Encode a the module name for the given source URL. When calculating a module name, we calculate the base 64 encoded
// sha1 of the entire source URL without the query string. For remote URLs (e.g. Git URLs), this is based on the
// assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar) identifies the module name and the
// query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no query string, so the same
// file path (/foo/bar) is always considered the same version. See also the encodeSourceVersion and
// processTerraformSource methods.
func encodeSourceName(sourceUrl *url.URL) (string, error) {
	sourceUrlNoQuery, err := parseSourceUrl(sourceUrl.String())
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	sourceUrlNoQuery.RawQuery = ""

	return util.EncodeBase64Sha1(sourceUrlNoQuery.String()), nil
}

// Returns true if the given URL refers to a path on the local file system
func isLocalSource(sourceUrl *url.URL) bool {
	return sourceUrl.Scheme == "file"
}

// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the Terragrunt configuration. If the user used one of these, this
// method returns the source URL or an empty string if there is no source url
func getTerraformSourceUrl(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) string {
	if terragruntOptions.Source != "" {
		return terragruntOptions.Source
	} else if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil {
		return *terragruntConfig.Terraform.Source
	} else {
		return ""
	}
}

// We use this code to force go-getter to copy files instead of creating symlinks.
var copyFiles = func(client *getter.Client) error {

	// We copy all the default getters from the go-getter library, but replace the "file" getter. We shallow clone the
	// getter map here rather than using getter.Getters directly because (a) we shouldn't change the original,
	// globally-shared getter.Getters map and (b) Terragrunt may run this code from many goroutines concurrently during
	// xxx-all calls, so creating a new map each time ensures we don't a "concurrent map writes" error.
	client.Getters = map[string]getter.Getter{}
	for getterName, getterValue := range getter.Getters {
		if getterName == "file" {
			client.Getters[getterName] = &FileCopyGetter{}
		} else {
			client.Getters[getterName] = getterValue
		}
	}

	return nil
}

// Download the code from the Canonical Source URL into the Download Folder using the go-getter library
func downloadSource(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) error {
	terragruntOptions.Logger.Printf("Downloading Terraform configurations from %s into %s", terraformSource.CanonicalSourceURL, terraformSource.DownloadDir)

	if err := getter.GetAny(terraformSource.DownloadDir, terraformSource.CanonicalSourceURL.String(), copyFiles); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
