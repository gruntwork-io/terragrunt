package tf

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

// ModuleVersionsErr is returned if we failed to fetch the module versions from the registry.
type ModuleVersionsErr struct {
	moduleName string
}

func (err ModuleVersionsErr) Error() string {
	return fmt.Sprintf("Failed to fetch versions for module %s. Ensure you're properly authenticated and your registry is reachable", err.moduleName)
}

// ModuleVersionConstraintErr is returned if the version constraint is not satisfied. This means there are no
// available versions for the module that satisfy the constraint.
type ModuleVersionConstraintErr struct {
	versionConstraint string
}

func (err ModuleVersionConstraintErr) Error() string {
	return fmt.Sprintf("Version constraint %s not satisfied", err.versionConstraint)
}

// ModuleVersionConstraintMalformedErr is returned if the version constraint is malformed and cannot be parsed.
type ModuleVersionConstraintMalformedErr struct {
	versionConstraint string
}

func (err ModuleVersionConstraintMalformedErr) Error() string {
	return fmt.Sprintf("Version constraint %s is malformed and cannot be parsed", err.versionConstraint)
}

// ModuleVersionMalformedErr is returned if the version string is malformed and cannot be parsed.
type ModuleVersionMalformedErr struct {
	version string
}

func (err ModuleVersionMalformedErr) Error() string {
	return fmt.Sprintf("Version %s is malformed and cannot be parsed", err.version)
}
