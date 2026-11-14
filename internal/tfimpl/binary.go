package tfimpl

import "github.com/gruntwork-io/terragrunt/internal/vexec"

const (
	// TofuDefaultPath is the name of the OpenTofu binary, looked up on PATH.
	TofuDefaultPath = "tofu"
	// TerraformDefaultPath is the name of the Terraform binary, looked up on PATH.
	TerraformDefaultPath = "terraform"
)

// DefaultBinary returns the binary Terragrunt wraps when nothing names one,
// preferring OpenTofu and falling back to Terraform.
//
// The choice comes from a PATH lookup alone. Spawning the candidate to confirm
// it runs costs more than the rest of startup combined.
func DefaultBinary(e vexec.Exec) string {
	if _, err := e.LookPath(TofuDefaultPath); err == nil {
		return TofuDefaultPath
	}

	return TerraformDefaultPath
}

// BinaryOrDefault returns tfPath, the binary a user or a unit named, or
// [DefaultBinary] when tfPath is empty.
func BinaryOrDefault(e vexec.Exec, tfPath string) string {
	if tfPath != "" {
		return tfPath
	}

	return DefaultBinary(e)
}
