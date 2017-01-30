package cli

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/config"
	"os"
	"github.com/gruntwork-io/terragrunt/errors"
	"path/filepath"
	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"
	"io/ioutil"
	"net/url"
	"fmt"
)

type TerraformSource struct {
	// A canonical version of RawSource, in URL format
	CanonicalSourceURL *url.URL

	// The folder where we should download the source to
	DownloadDir         string

	// The path to a file in DownloadDir that stores the version number of the code
	VersionFile         string
}

func (src *TerraformSource) String() string {
	return fmt.Sprintf("TerraformSource{CanonicalSourceURL = %v, DownloadDir = %v, VersionFile = %v}", src.CanonicalSourceURL, src.DownloadDir, src.VersionFile)
}

// 1. Download the given source URL, which should use Terraform's module source syntax, into a temporary folder
// 2. Copy the contents of terragruntOptions.WorkingDir into the temporary folder.
// 3. Set terragruntOptions.WorkingDir to the temporary folder.
//
// See the processTerraformSource method for how we determine the temporary folder so we can reuse it across multiple
// runs of Terragrunt to avoid downloading everything from scratch every time.
func downloadTerraformSource(source string, terragruntOptions *options.TerragruntOptions) error {
	terraformSource, err := processTerraformSource(source, terragruntOptions)
	if err != nil {
		return err
	}

	if err := downloadTerraformSourceIfNecessary(terraformSource, terragruntOptions); err != nil {
		return err
	}
	
	terragruntOptions.Logger.Printf("Copying files from %s into %s", terragruntOptions.WorkingDir, terraformSource.DownloadDir)
	if err := util.CopyFolderContents(terragruntOptions.WorkingDir, terraformSource.DownloadDir); err != nil {
		return err
	}

	terragruntOptions.Logger.Printf("Setting working directory to %s", terraformSource.DownloadDir)
	terragruntOptions.WorkingDir = terraformSource.DownloadDir

	return nil
}

// Download the specified TerraformSource if the latest code hasn't already been downloaded.
func downloadTerraformSourceIfNecessary(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions) error {
	alreadyLatest, err := alreadyHaveLatestCode(terraformSource)
	if err != nil {
		return err
	}

	if alreadyLatest {
		terragruntOptions.Logger.Printf("Terraform files in %s are up to date. Will not download again.", terraformSource.DownloadDir)
		return nil
	}

	if err := cleanupTerraformFiles(terraformSource.DownloadDir, terragruntOptions); err != nil {
		return err
	}

	if err := terraformInit(terraformSource, terragruntOptions); err != nil {
		return err
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
func alreadyHaveLatestCode(terraformSource *TerraformSource) (bool, error) {
	if 	isLocalSource(terraformSource.CanonicalSourceURL) ||
		!util.FileExists(terraformSource.DownloadDir) ||
		!util.FileExists(terraformSource.VersionFile) {

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
// To maximize reuse, given a working directory w and a source URL s, we download the code into the folder /T/W/S where:
//
// 1. T is the OS temp dir (e.g. /tmp).
// 2. W is the base 64 encoded sha1 hash of w. This ensures that if you are running Terragrunt concurrently in
//    multiple folders (e.g. during automated tests), then even if those folders are using the same source URL s, they
//    do not overwrite each other.
// 3. S is the base 64 encoded sha1 has of s without its query string. For remote source URLs (e.g. Git
//    URLs), this is based on the assumption that the scheme/host/path of the URL 
//    (e.g. git::github.com/foo/bar//some-module) identifies the module name, and we always want to download the same
//    module name into the same folder (see the encodeSourceName method). We also assume the version of the module is
//    stored in the query string (e.g. ref=v0.0.3), so we store the base 64 encoded sha1 of the query string in a
//    file called .terragrunt-source-version within S.
//
// The downloadTerraformSourceIfNecessary decides when we should download the Terraform code and when not to. It uses
// the following rules:
//
// 1. Always download source URLs pointing to local file paths.
// 2. Only download source URLs pointing to remote paths if /T/W/S doesn't already exist or, if it does exist, if the
//    version number in /T/W/S/.terragrunt-source-version doesn't match the current version.
func processTerraformSource(source string, terragruntOptions *options.TerragruntOptions) (*TerraformSource, error) {
	canonicalWorkingDir, err := util.CanonicalPath(terragruntOptions.WorkingDir, "")
	if err != nil {
		return nil, err
	}

	rawSourceUrl, err := getter.Detect(source, canonicalWorkingDir, getter.Detectors)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	canonicalSourceUrl, err := urlhelper.Parse(rawSourceUrl)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if isLocalSource(canonicalSourceUrl) {
		// Always use canonical file paths for local source folders, rather than relative paths, to ensure
		// that the same local folder always maps to the same download folder, no matter how the local folder
		// path is specified
		canonicalFilePath, err := util.CanonicalPath(canonicalSourceUrl.Path, "")
		if err != nil {
			return nil, err
		}
		canonicalSourceUrl.Path = canonicalFilePath
	}

	moduleName, err := encodeSourceName(canonicalSourceUrl)
	if err != nil {
		return nil, err
	}

	encodedWorkingDir := util.EncodeBase64Sha1(canonicalWorkingDir)
	downloadDir := filepath.Join(os.TempDir(), "terragrunt-download", encodedWorkingDir, moduleName)
	versionFile := filepath.Join(downloadDir, ".terragrunt-source-version")

	return &TerraformSource{
		CanonicalSourceURL: canonicalSourceUrl,
		DownloadDir: downloadDir,
		VersionFile: versionFile,
	}, nil
}

// Encode a version number for the given source URL. When calculating a version number, we simply take the query
// string of the source URL, calculate its sha1, and base 64 encode it. For remote URLs (e.g. Git URLs), this is
// based on the assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar//some-module) identifies
// the module name and the query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no
// query string, so the same file path (/foo/bar) is always considered the same version. See also the encodeSourceName
// and processTerraformSource methods.
func encodeSourceVersion(sourceUrl *url.URL) string {
	return util.EncodeBase64Sha1(sourceUrl.Query().Encode())
}

// Encode a the module name for the given source URL. When calculating a module name, we calculate the base 64 encoded
// sha1 of the entire source URL without the query string. For remote URLs (e.g. Git URLs), this is based on the
// assumption that the scheme/host/path of the URL (e.g. git::github.com/foo/bar//some-module) identifies
// the module name and the query string (e.g. ?ref=v0.0.3) identifies the version. For local file paths, there is no
// query string, so the same file path (/foo/bar) is always considered the same version. See also the encodeSourceVersion
// and processTerraformSource methods.
func encodeSourceName(sourceUrl *url.URL) (string, error) {
	sourceUrlNoQuery, err := urlhelper.Parse(sourceUrl.String())
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

// If this temp folder already exists, simply delete all the Terraform configurations (*.tf) within it
// (the terraform init command will redownload the latest ones), but leave all the other files, such
// as the .terraform folder with the downloaded modules and remote state settings.
func cleanupTerraformFiles(path string, terragruntOptions *options.TerragruntOptions) error {
	if !util.FileExists(path) {
		return nil
	}

	terragruntOptions.Logger.Printf("Cleaning up existing *.tf files in %s", path)

	files, err := filepath.Glob(filepath.Join(path, "*.tf"))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	return util.DeleteFiles(files)
}

// There are two ways a user can tell Terragrunt that it needs to download Terraform configurations from a specific
// URL: via a command-line option or via an entry in the .terragrunt config file. If the user used one of these, this
// method returns the source URL and the boolean true; if not, this method returns an empty string and false.
func getTerraformSourceUrl(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) (string, bool) {
	if terragruntOptions.Source != "" {
		return terragruntOptions.Source, true
	} else if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != "" {
		return terragruntConfig.Terraform.Source, true
	} else {
		return "", false
	}
}

// Download the code from the Canonical Source URL into the Download Folder using the terraform init command
func terraformInit(terraformSource *TerraformSource, terragruntOptions *options.TerragruntOptions) error {
	terragruntOptions.Logger.Printf("Downloading Terraform configurations from %s into %s", terraformSource.CanonicalSourceURL, terraformSource.DownloadDir)

	terragruntInitOptions := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntInitOptions.TerraformCliArgs = []string{"init", terraformSource.CanonicalSourceURL.String(), terraformSource.DownloadDir}

	return runTerraformCommand(terragruntInitOptions)
}
