package test_test

import (
	"bytes"
	"context"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureBackendUnappliedDependency = "fixtures/backend-unapplied-dependency"

func TestBackendBootstrapWithUnappliedDependency(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureBackendUnappliedDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureBackendUnappliedDependency)
	unitPath := filepath.Join(tmpEnvPath, testFixtureBackendUnappliedDependency, "foo")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt backend bootstrap --non-interactive --working-dir "+unitPath,
	)
	require.NoError(t, err)
}

func TestBackendAllReadsFilterFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		command  string
		file     string
		contents string
	}{
		{
			name:     "bootstrap with a filters file",
			command:  "backend bootstrap --all",
			file:     ".terragrunt-filters",
			contents: "a\n",
		},
		{
			name:     "bootstrap with an excludes file",
			command:  "backend bootstrap --all",
			file:     ".terragrunt-excludes",
			contents: "b\n",
		},
		{
			name:     "delete with a filters file",
			command:  "backend delete --all --force",
			file:     ".terragrunt-filters",
			contents: "a\n",
		},
		{
			name:     "delete with an excludes file",
			command:  "backend delete --all --force",
			file:     ".terragrunt-excludes",
			contents: "b\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			files := map[string]string{
				filepath.Join("a", "terragrunt.hcl"): `remote_state {
  backend = "local"
  config = {
    path = "terraform.tfstate"
  }
}
`,
				filepath.Join("b", "terragrunt.hcl"): "this is ) not ( valid hcl",
				tc.file:                              tc.contents,
			}

			for name, contents := range files {
				path := filepath.Join(tmpDir, name)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt "+tc.command+" --non-interactive --working-dir "+tmpDir,
			)
			require.NoError(t, err, "b fails to parse if the command reaches it\nstdout: %s\nstderr: %s", stdout, stderr)
		})
	}
}

// TestBackendRequireBootstrap runs each command that bootstraps remote state with
// --backend-require-bootstrap against an S3 that has no state bucket.
func TestBackendRequireBootstrap(t *testing.T) {
	t.Parallel()

	const bucket = "terragrunt-test-missing-bucket"

	testCases := []struct {
		name    string
		command string
		iacArgs string
	}{
		{
			name:    "backend bootstrap",
			command: "backend bootstrap",
		},
		{
			name:    "run with backend bootstrap",
			command: "run --backend-bootstrap",
			iacArgs: " -- plan",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			unitPath := helpers.TmpDirWOSymlinks(t)
			config := `remote_state {
  backend = "s3"
  config = {
    bucket                      = "` + bucket + `"
    key                         = "terraform.tfstate"
    region                      = "us-east-1"
    endpoints                   = { s3 = "https://s3.example.test" }
    use_path_style              = true
    skip_credentials_validation = true
  }
}
`
			require.NoError(t, os.WriteFile(filepath.Join(unitPath, "terragrunt.hcl"), []byte(config), 0o644))
			require.NoError(t, os.WriteFile(
				filepath.Join(unitPath, "main.tf"),
				[]byte("terraform {\n  backend \"s3\" {}\n}\n"),
				0o644,
			))

			var (
				mu             sync.Mutex
				changeRequests []string
			)

			// The bucket does not exist, so S3 answers HeadBucket with 404, which has no body.
			s3 := func(_ context.Context, req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodHead || req.Method == http.MethodGet {
					return vhttp.Respond(http.StatusNotFound, nil, nil), nil
				}

				mu.Lock()
				defer mu.Unlock()

				changeRequests = append(changeRequests, req.Method+" "+req.URL.String())

				return vhttp.Respond(http.StatusNotImplemented, nil, nil), nil
			}

			// Terragrunt asks for the version before it bootstraps. The bootstrap fails, so no other tofu
			// command may run.
			tofu := func(_ context.Context, inv vexec.Invocation) vexec.Result {
				if slices.Equal(inv.Args, []string{"-version"}) {
					return vexec.Result{Stdout: []byte("OpenTofu v1.10.0\n")}
				}

				return vexec.Result{ExitCode: 1, Stderr: []byte("unexpected tofu " + strings.Join(inv.Args, " "))}
			}

			v := venv.OSVenv().
				WithHTTP(vhttp.NewMemClient(s3)).
				WithExec(vexec.NewMemExec(tofu, vexec.WithLookPath(func(file string) (string, error) {
					return filepath.Join("/usr/bin", file), nil
				}))).
				WithWriter(&bytes.Buffer{}).
				WithErrWriter(&bytes.Buffer{})

			maps.DeleteFunc(v.Env, func(k, _ string) bool { return strings.HasPrefix(k, "AWS_") })
			v.Env["AWS_ACCESS_KEY_ID"] = "fake-access-key-id"
			v.Env["AWS_SECRET_ACCESS_KEY"] = "fake-secret-access-key"

			err := helpers.RunTerragruntCommandWithVenv(
				t,
				t.Context(),
				v,
				"terragrunt "+tc.command+" --backend-require-bootstrap --non-interactive --working-dir "+unitPath+tc.iacArgs,
			)

			var notAllowed backend.BucketCreationNotAllowed

			require.ErrorAs(t, err, &notAllowed)
			assert.Equal(t, backend.BucketCreationNotAllowed(bucket), notAllowed)
			assert.Empty(t, changeRequests, "no request may create or change a bucket")
		})
	}
}
