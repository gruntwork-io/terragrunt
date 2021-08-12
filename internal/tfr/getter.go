package tfr

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/go-getter"
	safetemp "github.com/hashicorp/go-safetemp"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// httpClient is the default client to be used by HttpGetters.
var httpClient = cleanhttp.DefaultClient()

// Constants relevant to the module registry
const (
	serviceDiscoveryPath = "/.well-known/terraform.json"
	sdModulesKey         = "modules.v1"
	versionQueryKey      = "version"
	authTokenEnvVarName  = "TG_TF_REGISTRY_TOKEN"
)

// TerraformRegistryGetter is a Getter (from go-getter) implementation that will download from the terraform module
// registry. This supports getter URLs encoded in the following manner:
//
// tfr://REGISTRY_DOMAIN/MODULE_PATH?version=VERSION
//
// Where the REGISTRY_DOMAIN is the terraform registry endpoint (e.g., registry.terraform.io), MODULE_PATH is the
// registry path for the module (e.g., terraform-aws-modules/vpc/aws), and VERSION is the specific version of the module
// to download (e.g., 2.2.0).
//
// This protocol will use the Module Registry Protocol (documented at
// https://www.terraform.io/docs/internals/module-registry-protocol.html) to lookup the module source URL and download
// it.
//
// Authentication to private module registries is handled via environment variables. The authorization API token is
// expected to be provided to Terragrunt via the TG_TF_REGISTRY_TOKEN environment variable.
// TODO: expand on what this token is and how one should get it
//
// MAINTAINER'S NOTE: Ideally we implement the full credential system that terraform uses as part of login, but all the
// relevant packages are internal to the terraform repository, thus making it difficult to use as a library. For now, we
// keep things simple by supporting providing tokens via env vars and in the future, we can consider implementing
// functionality to load credentials from terraform.
type TerraformRegistryGetter struct {
	client *getter.Client
}

// SetClient allows the getter to know what getter client (different from the underlying HTTP client) to use for
// progress tracking.
func (tfrGetter *TerraformRegistryGetter) SetClient(client *getter.Client) {
	tfrGetter.client = client
}

// Context returns the go context to use for the underlying fetch routines. This depends on what client is set.
func (tfrGetter *TerraformRegistryGetter) Context() context.Context {
	if tfrGetter == nil || tfrGetter.client == nil {
		return context.Background()
	}
	return tfrGetter.client.Ctx
}

// ClientMode returns the download mode based on the given URL. Since this getter is designed around the Terraform
// module registry, we always use Dir mode so that we can download the full Terraform module.
func (tfrGetter *TerraformRegistryGetter) ClientMode(u *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

// Get is the main routine to fetch the module contents specified at the given URL and download it to the dstPath.
// This routine assumes that the srcURL points to the Terraform registry URL, with the Path configured to the module
// path encoded as `:namespace/:name/:system` as expected by the Terraform registry. Note that the URL query parameter
// must have the `version` key to specify what version to download.
func (tfrGetter *TerraformRegistryGetter) Get(dstPath string, srcURL *url.URL) error {
	ctx := tfrGetter.Context()

	registryDomain := srcURL.Host
	queryValues := srcURL.Query()
	modulePath, moduleSubDir := getter.SourceDirSubdir(srcURL.Path)

	versionList, hasVersion := queryValues[versionQueryKey]
	if !hasVersion {
		err := fmt.Errorf("tfr getter URL missing version query")
		return errors.WithStackTrace(err)
	}
	if len(versionList) != 1 {
		err := fmt.Errorf("tfr getter URL has more than one version query")
		return errors.WithStackTrace(err)
	}
	version := versionList[0]

	moduleRegistryBasePath, err := getModuleRegistryURLBasePath(ctx, registryDomain)
	if err != nil {
		return err
	}

	moduleFullPath := path.Join(moduleRegistryBasePath, modulePath, version, "download")
	moduleURL := url.URL{
		Scheme: "https",
		Host:   registryDomain,
		Path:   moduleFullPath,
	}
	downloadURL, err := getDownloadURLFromRegistry(ctx, moduleURL)
	if err != nil {
		return err
	}

	// If there is a subdir component, then we download the root separately into a temporary directory, then copy over
	// the proper subdir. Note that we also have to take into account sub dirs in the original URL in addition to the
	// subdir component in the X-Terraform-Get download URL.
	source, subDir := getter.SourceDirSubdir(downloadURL)
	if subDir == "" && moduleSubDir == "" {
		var opts []getter.ClientOption
		if tfrGetter.client != nil {
			opts = tfrGetter.client.Options
		}
		return getter.Get(dstPath, source, opts...)
	}

	// We have a subdir, time to jump some hoops
	return tfrGetter.getSubdir(ctx, dstPath, source, path.Join(subDir, moduleSubDir))
}

// GetFile is not implemented for the Terraform module registry Getter since the terraform module registry doesn't serve
// a single file.
func (tfrGetter *TerraformRegistryGetter) GetFile(dst string, src *url.URL) error {
	return errors.WithStackTrace(fmt.Errorf("GetFile is not implemented for the Terraform Registry Getter"))
}

// getSubdir downloads the source into the destination, but with the proper subdir.
func (tfrGetter *TerraformRegistryGetter) getSubdir(ctx context.Context, dstPath, sourceURL, subDir string) error {
	// Create a temporary directory to store the full source. This has to be a non-existent directory.
	tempdirPath, tempdirCloser, err := safetemp.Dir("", "getter")
	if err != nil {
		return err
	}
	defer tempdirCloser.Close()

	var opts []getter.ClientOption
	if tfrGetter.client != nil {
		opts = tfrGetter.client.Options
	}
	// Download that into the given directory
	if err := getter.Get(tempdirPath, sourceURL, opts...); err != nil {
		return errors.WithStackTrace(err)
	}

	// Process any globbing
	sourcePath, err := getter.SubdirGlob(tempdirPath, subDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	// Make sure the subdir path actually exists
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("Error downloading %s: %s", sourceURL, err)
	}

	// Copy the subdirectory into our actual destination.
	if err := os.RemoveAll(dstPath); err != nil {
		return errors.WithStackTrace(err)
	}

	// Make the final destination
	if err := os.MkdirAll(dstPath, 0755); err != nil {
		return errors.WithStackTrace(err)
	}

	// We use a temporary manifest file here that is deleted at the end of this routine since we don't intend to come
	// back to it.
	manifestFname := ".tgmanifest"
	manifestPath := filepath.Join(dstPath, manifestFname)
	defer os.Remove(manifestPath)
	return util.CopyFolderContentsWithFilter(sourcePath, dstPath, manifestFname, func(path string) bool { return true })
}

// getModuleRegistryURLBasePath uses the service discovery protocol
// (https://www.terraform.io/docs/internals/remote-service-discovery.html)
// to figure out where the modules are stored. This will return the base
// path where the modules can be accessed
func getModuleRegistryURLBasePath(ctx context.Context, domain string) (string, error) {
	sdURL := url.URL{
		Scheme: "https",
		Host:   domain,
		Path:   serviceDiscoveryPath,
	}
	bodyData, _, err := httpGETAndGetResponse(ctx, sdURL)
	if err != nil {
		return "", err
	}

	var respJSON map[string]interface{}
	if err := json.Unmarshal(bodyData, &respJSON); err != nil {
		return "", errors.WithStackTrace(err)
	}

	modulePathRaw, hasModulePath := respJSON[sdModulesKey]
	if !hasModulePath {
		err := fmt.Errorf("response body does not contain modules.v1 key: %s", string(bodyData))
		return "", errors.WithStackTrace(err)
	}
	modulePath, isString := modulePathRaw.(string)
	if !isString {
		err := fmt.Errorf("modules.v1 key is not a string: %s", string(bodyData))
		return "", errors.WithStackTrace(err)
	}
	return modulePath, nil
}

// getDownloadURLFromRegistry makes an http GET call to the given registry URL and return the contents of the header
// X-Terraform-Get. This function will return an error if the response does not contain the header.
func getDownloadURLFromRegistry(ctx context.Context, url url.URL) (string, error) {
	_, header, err := httpGETAndGetResponse(ctx, url)
	if err != nil {
		return "", err
	}
	terraformGet := header.Get("X-Terraform-Get")
	if terraformGet == "" {
		err := fmt.Errorf("no source URL was returned from download URL %s", url.String())
		return "", errors.WithStackTrace(err)
	}
	return terraformGet, nil
}

// httpGETAndGetResponse is a helper function to make a GET request to the given URL using the http client. This
// function will then read the response and return the contents + the response header.
func httpGETAndGetResponse(ctx context.Context, getURL url.URL) ([]byte, *http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", getURL.String(), nil)
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}

	// Handle authentication via env var. Authentication is done by providing the registry token as a bearer token in
	// the request header.
	authToken := os.Getenv(authTokenEnvVarName)
	if authToken != "" {
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", authToken))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}

	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("failed to fetch url %s: %d", getURL.String(), resp.StatusCode)
		return nil, nil, errors.WithStackTrace(err)
	}

	bodyData, err := ioutil.ReadAll(resp.Body)
	return bodyData, &resp.Header, errors.WithStackTrace(err)
}
