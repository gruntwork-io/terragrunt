//go:build tofu

package test_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAutoProviderCacheDir = "fixtures/auto-provider-cache-dir"
	testFixtureTfPathDependency     = "fixtures/tf-path/dependency"
	testFixtureTofuHTTPEncryption   = "fixtures/tofu-http-encryption"
)

func TestAutoProviderCacheDirExperimentBasic(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := filepath.Join(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(t, stderr, "using cache key for version files")
	assert.Contains(t, stderr, "Auto provider cache dir enabled")
	assert.Regexp(t, `(Reusing previous version|shared cache directory)`, stdout)
}

func TestAutoProviderCacheDirExperimentRunAll(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := filepath.Join(testPath, "basic", "unit")

	// clone the unit dir 9 times
	for i := range 9 {
		helpers.CopyDir(t, unitPath, filepath.Join(testPath, "unit-"+strconv.Itoa(i)))
	}

	cmd := "terragrunt run --all init --log-level debug --non-interactive --working-dir " + testPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Auto provider cache dir enabled")
	assert.Contains(t, stderr, "using cache key for version files")
	assert.Regexp(t, `(Reusing previous version|shared cache directory)`, stdout)
}

func TestAutoProviderCacheDirDisabled(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAutoProviderCacheDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureAutoProviderCacheDir)
	unitPath := filepath.Join(testPath, "basic", "unit")

	cmd := "terragrunt init --log-level debug --no-auto-provider-cache-dir --non-interactive --working-dir " + unitPath

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "Auto provider cache dir enabled")
	assert.NotRegexp(t, `Using hashicorp\/null [^ ]+ from the shared cache directory`, stdout)
}

func TestTfPathRespectedForDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureTfPathDependency)
	rootPath := helpers.CopyEnvironment(t, testFixtureTfPathDependency)
	testPath := filepath.Join(rootPath, testFixtureTfPathDependency)
	testPath, err := filepath.EvalSymlinks(testPath)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		fmt.Sprintf(
			"terragrunt run --all --non-interactive --tf-path %s --working-dir %s -- apply",
			filepath.Join(testPath, "custom-tf.sh"),
			testPath,
		),
	)
	require.NoError(
		t,
		err,
		"Expected tf-path to be respected for dependency lookups, but it was overridden by terraform_binary in config",
	)
	assert.Contains(t, stderr, "Custom TF script used in ./app")
	assert.Contains(t, stderr, "Custom TF script used in ./dep")
}

// TestHTTPBackendEncryptionDependencyFails tests that OpenTofu state encryption
// with HTTP backend works correctly when reading dependency outputs.
func TestHTTPBackendEncryptionDependencyFails(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	serverURL := runHTTPStateServer(t, ctx)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTofuHTTPEncryption)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testFixtureTofuHTTPEncryption)

	rootHclPath := filepath.Join(testPath, "root.hcl")
	rootHclContent, err := os.ReadFile(rootHclPath)
	require.NoError(t, err)

	newContent := strings.ReplaceAll(string(rootHclContent), "__HTTP_SERVER_URL__", serverURL)
	err = os.WriteFile(rootHclPath, []byte(newContent), 0644)
	require.NoError(t, err)

	cmd := "terragrunt run --all apply --non-interactive --working-dir " + testPath
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err)

	assert.NotContains(t, stderr, "This state file is encrypted and can not be read without an encryption configuration")
}

// runHTTPStateServer starts an HTTP server that implements the Terraform HTTP backend API.
// It returns the server URL.
func runHTTPStateServer(t *testing.T, ctx context.Context) string {
	t.Helper()

	var (
		states = make(map[string][]byte)
		locks  = make(map[string]string)
		mu     sync.RWMutex
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/state/", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		const prefix = "Basic "
		if !strings.HasPrefix(auth, prefix) {
			http.Error(w, "Invalid auth header", http.StatusUnauthorized)
			return
		}

		decoded, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
		if err != nil {
			http.Error(w, "Invalid auth encoding", http.StatusUnauthorized)
			return
		}

		credentials := string(decoded)
		if credentials != "admin:secret" {
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		path := r.URL.Path

		switch r.Method {
		case http.MethodGet:
			mu.RLock()

			state, ok := states[path]

			mu.RUnlock()

			if !ok {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Write(state)

		case http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()

			states[path] = body

			mu.Unlock()

			w.WriteHeader(http.StatusOK)

		case "LOCK":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var lockInfo struct {
				ID string `json:"ID"`
			}
			if err := json.Unmarshal(body, &lockInfo); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()

			if existingLock, ok := locks[path]; ok {
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusLocked)
				json.NewEncoder(w).Encode(map[string]string{"ID": existingLock})

				return
			}

			locks[path] = lockInfo.ID

			mu.Unlock()

			w.WriteHeader(http.StatusOK)

		case "UNLOCK":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var lockInfo struct {
				ID string `json:"ID"`
			}
			if err := json.Unmarshal(body, &lockInfo); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			mu.Lock()
			delete(locks, path)
			mu.Unlock()

			w.WriteHeader(http.StatusOK)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	var lc net.ListenConfig

	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{
		Handler: mux,
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("HTTP server error: %v", err)
		}
	}()

	go func() {
		<-ctx.Done()
		server.Shutdown(context.WithoutCancel(ctx))
	}()

	return "http://" + listener.Addr().String()
}
