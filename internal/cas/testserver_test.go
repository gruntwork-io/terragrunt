package cas_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/stretchr/testify/require"
)

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
