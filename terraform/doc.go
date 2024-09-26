// Package terraform contains functions and routines for interacting with OpenTofu/Terraform.
//
// Package tfr contains functions and routines for interacting with terraform registries using the Module Registry
// Protocol documented in the official terraform docs:
// https://www.terraform.io/docs/internals/module-registry-protocol.html
//
// MAINTAINER'S NOTE: Ideally we would be able to reuse code from Terraform. However, terraform has moved to packaging
// all its libraries under internal so that you can't use them as a library outside of Terraform. To respect the
// direction and spirit of the Terraform team, we opted for not doing anything funky to workaround the limitation (like
// copying those files in here). We also opted to keep this functionality internal to align with the Terraform team's
// decision to not support client libraries for accessing the Terraform Registry.
package terraform
