package getter

// OCI manifest-contract constants that must match OpenTofu's OCI module
// implementation exactly so a single oci:// source string stays portable
// between tofu and Terragrunt, making them a compatibility contract rather
// than implementation details to change freely.
const (
	ArtifactTypeModulePkg = "application/vnd.opentofu.modulepkg"
	MediaTypeModuleZip    = "archive/zip"
)
