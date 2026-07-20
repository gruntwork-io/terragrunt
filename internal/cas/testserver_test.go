package cas_test

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

// TestMain shuts down the fixture servers shared by startTestServer and
// startStackTestServer, if any test started them, once the package's
// tests finish.
func TestMain(m *testing.M) {
	defer closeSharedServers()

	m.Run()
}

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

// startTestServer returns the URL of a package-shared Git server
// populated once with a few test files. The fixture is immutable and
// read-only for every caller; tests that need a mutable server must use
// newEmptyTestServer. Each call returns a distinct URL that routes to
// the same repository, so tests that partition state by URL stay
// isolated from each other.
func startTestServer(t *testing.T) string {
	t.Helper()

	srv, err := sharedTestServer()
	require.NoError(t, err)

	return uniqueRepoURL(srv)
}

// startStackTestServer returns the URL of a package-shared Git server
// populated once with a realistic stack repository structure. The
// fixture is immutable and read-only for every caller; tests that need
// a mutable server must use newEmptyTestServer. Each call returns a
// distinct URL that routes to the same repository, so tests that
// partition state by URL stay isolated from each other.
//
// The repo layout is:
//
//	stacks/my-stack/terragrunt.stack.hcl   stack file with update_source_with_cas
//	units/my-service/terragrunt.hcl        unit with `//` terraform.source
//	units/leaf-service/terragrunt.hcl      unit with no-`//` terraform.source
//	modules/vpc/main.tf                    plain Terraform module
//	modules/sibling/main.tf                sibling module reachable from vpc via "../sibling"
func startStackTestServer(t *testing.T) string {
	t.Helper()

	srv, err := sharedStackTestServer()
	require.NoError(t, err)

	return uniqueRepoURL(srv)
}

// sharedTestServer and sharedStackTestServer build their fixture server
// on first use and return the cached server (or construction error) to
// every later caller.
var (
	sharedTestServer      = sync.OnceValues(newSharedTestServer)
	sharedStackTestServer = sync.OnceValues(newSharedStackTestServer)
)

// sharedServers tracks the fixture servers started by the shared
// helpers so closeSharedServers can shut them down after the package's
// tests finish.
var (
	sharedServersMu sync.Mutex
	sharedServers   []*git.Server

	// sharedRepoURLID feeds uniqueRepoURL with a fresh path component
	// per call.
	sharedRepoURLID atomic.Int64
)

// newSharedTestServer builds the fixture repository behind
// startTestServer.
func newSharedTestServer() (*git.Server, error) {
	files := map[string][]byte{
		"README.md":                []byte("# test repo"),
		"main.tf":                  []byte(`resource "null_resource" "test" {}`),
		"test/integration_test.go": []byte("package test"),
	}

	return newSharedFixtureServer(files, "add test fixture")
}

// newSharedStackTestServer builds the fixture repository behind
// startStackTestServer.
func newSharedStackTestServer() (*git.Server, error) {
	// Stack file that references sibling units via relative paths with //.
	stackHCL := []byte(`unit "service" {
  source = "../..//units/my-service"

  update_source_with_cas = true

  path = "service"
}

unit "leaf" {
  source = "../..//units/leaf-service"

  update_source_with_cas = true

  path = "leaf"
}

unit "plain" {
  source = "../../units/plain-service"
  path   = "plain"
}
`)

	// Unit file whose terraform.source references a module via "//" so the
	// synthetic tree retains the surrounding repo structure.
	unitHCL := []byte(`terraform {
  source = "../..//modules/vpc"

  update_source_with_cas = true
}
`)

	// Unit file whose terraform.source omits "//". The synthetic tree must
	// stay scoped to the leaf module and the rewritten ref must carry no
	// "//subdir" tail.
	leafUnitHCL := []byte(`terraform {
  source = "../../modules/vpc"

  update_source_with_cas = true
}
`)

	// Plain unit (no CAS flag) should remain unchanged after processing.
	plainUnitHCL := []byte(`terraform {
  source = "../../modules/vpc"
}
`)

	// OpenTofu/Terraform module referenced by the unit. It cross-references a
	// sibling module via a relative path, which only resolves if the synthetic
	// tree retains the surrounding repo structure.
	vpcMainTF := []byte(`module "sibling" {
  source = "../sibling"
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`)

	vpcVariablesTF := []byte(`variable "name" {
  type = string
}
`)

	// Sibling module that the vpc module references via "../sibling".
	siblingMainTF := []byte(`output "name" {
  value = "sibling"
}
`)

	files := map[string][]byte{
		"stacks/my-stack/terragrunt.stack.hcl": stackHCL,
		"units/my-service/terragrunt.hcl":      unitHCL,
		"units/leaf-service/terragrunt.hcl":    leafUnitHCL,
		"units/plain-service/terragrunt.hcl":   plainUnitHCL,
		"modules/vpc/main.tf":                  vpcMainTF,
		"modules/vpc/variables.tf":             vpcVariablesTF,
		"modules/sibling/main.tf":              siblingMainTF,
	}

	return newSharedFixtureServer(files, "add stack fixture")
}

// newSharedFixtureServer creates a Server, commits all files in one
// batch, starts serving, and registers the server for shutdown in
// closeSharedServers. The server outlives any single test, so it uses
// context.Background() rather than a test context.
func newSharedFixtureServer(files map[string][]byte, msg string) (*git.Server, error) {
	srv, err := git.NewServer()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	if err := srv.CommitFiles(ctx, files, msg); err != nil {
		_ = srv.Close()

		return nil, err
	}

	if _, err := srv.Start(ctx); err != nil {
		_ = srv.Close()

		return nil, err
	}

	registerSharedServer(srv)

	return srv, nil
}

// registerSharedServer records a started fixture server for shutdown in
// closeSharedServers.
func registerSharedServer(srv *git.Server) {
	sharedServersMu.Lock()
	defer sharedServersMu.Unlock()

	sharedServers = append(sharedServers, srv)
}

// uniqueRepoURL returns a fresh URL that routes to srv's single
// repository. The server rewrites unknown path components to its real
// repo, so every returned URL serves the same fixture while remaining a
// distinct URL identity.
func uniqueRepoURL(srv *git.Server) string {
	return srv.BaseURL() + "/repo-" + strconv.FormatInt(sharedRepoURLID.Add(1), 10) + ".git"
}

// closeSharedServers shuts down every fixture server started by the
// shared helpers. Called from TestMain after all tests complete.
func closeSharedServers() {
	sharedServersMu.Lock()
	defer sharedServersMu.Unlock()

	for _, srv := range sharedServers {
		_ = srv.Close()
	}
}
