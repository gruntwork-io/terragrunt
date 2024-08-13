package terraform

import "fmt"

// MalformedRegistryURLErr is returned if the Terraform Registry URL passed to the Getter is malformed.
type MalformedRegistryURLErr struct {
	reason string
}

func (err MalformedRegistryURLErr) Error() string {
	return "tfr getter URL is malformed: " + err.reason
}

// ServiceDiscoveryErr is returned if Terragrunt failed to identify the module API endpoint through the service
// discovery protocol.
type ServiceDiscoveryErr struct {
	reason string
}

func (err ServiceDiscoveryErr) Error() string {
	return "Error identifying module registry API location: " + err.reason
}

// ModuleDownloadErr is returned if Terragrunt failed to download the module.
type ModuleDownloadErr struct {
	sourceURL string
	details   string
}

func (err ModuleDownloadErr) Error() string {
	return fmt.Sprintf("Error downloading module from %s: %s", err.sourceURL, err.details)
}

// RegistryAPIErr is returned if we get an unsuccessful HTTP return code from the registry.
type RegistryAPIErr struct {
	url        string
	statusCode int
}

func (err RegistryAPIErr) Error() string {
	return fmt.Sprintf("Failed to fetch url %s: status code %d", err.url, err.statusCode)
}
