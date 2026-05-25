package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

// newEmptyTestServer creates a Server that the caller can populate
// before starting. The returned cleanup is registered on the test, so
// the caller only needs to invoke Start. Useful for tests that need
// access to commit hashes, tags, or want to shut the server down
// mid-test to verify offline behavior.
func newEmptyTestServer(t *testing.T) *git.Server {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	return srv
}

// startTestServer creates a local Git server with a few test files and
// returns its URL. The server is shut down when the test completes.
func startTestServer(t *testing.T) string {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.CommitFile("README.md", []byte("# test repo"), "add readme"))
	require.NoError(t, srv.CommitFile("main.tf", []byte(`resource "null_resource" "test" {}`), "add main.tf"))
	require.NoError(t, srv.CommitFile("test/integration_test.go", []byte("package test"), "add test file"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	return url
}

// startStackTestServer creates a local Git server with a realistic stack
// repository structure and returns its URL.
//
// The repo layout is:
//
//	stacks/my-stack/terragrunt.stack.hcl   — stack file with update_source_with_cas
//	units/my-service/terragrunt.hcl        — unit file with update_source_with_cas
//	modules/vpc/main.tf                    — plain Terraform module
func startStackTestServer(t *testing.T) string {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	// Stack file that references a sibling unit via relative path with //.
	stackHCL := []byte(`unit "service" {
  source = "../..//units/my-service"

  update_source_with_cas = true

  path = "service"
}

unit "plain" {
  source = "../../units/plain-service"
  path   = "plain"
}
`)
	require.NoError(t, srv.CommitFile("stacks/my-stack/terragrunt.stack.hcl", stackHCL, "add stack file"))

	// Unit file whose terraform.source references a module via relative path.
	unitHCL := []byte(`terraform {
  source = "../..//modules/vpc"

  update_source_with_cas = true
}
`)
	require.NoError(t, srv.CommitFile("units/my-service/terragrunt.hcl", unitHCL, "add unit file"))

	// Plain unit (no CAS flag) — should remain unchanged after processing.
	plainUnitHCL := []byte(`terraform {
  source = "../../modules/vpc"
}
`)
	require.NoError(t, srv.CommitFile("units/plain-service/terragrunt.hcl", plainUnitHCL, "add plain unit"))

	// Terraform module referenced by the unit.
	require.NoError(t, srv.CommitFile("modules/vpc/main.tf", []byte(`resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`), "add vpc module"))

	require.NoError(t, srv.CommitFile("modules/vpc/variables.tf", []byte(`variable "name" {
  type = string
}
`), "add vpc variables"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	return url
}
