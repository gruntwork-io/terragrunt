package config_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDependencyStateDirectReadRoutesByBackendAndWorkspace(t *testing.T) {
	t.Parallel()

	accessKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	testCases := []struct {
		name          string
		backend       string
		backendConfig string
		workspace     string
		wantRequest   string
		enableAzure   bool
	}{
		{
			name:    "S3 default workspace",
			backend: "s3",
			backendConfig: `
        bucket              = "state-bucket"
        key                 = "service.tfstate"
        region              = "us-east-1"
        endpoint            = "https://s3.example.com"
        force_path_style    = true
        skip_credentials_validation = true`,
			wantRequest: "s3.example.com/state-bucket/service.tfstate",
		},
		{
			name:      "S3 named workspace",
			backend:   "s3",
			workspace: "production",
			backendConfig: `
        bucket               = "state-bucket"
        key                  = "service.tfstate"
        region               = "us-east-1"
        endpoint             = "https://s3.example.com"
        force_path_style     = true
        workspace_key_prefix = "workspaces"
        skip_credentials_validation = true`,
			wantRequest: "s3.example.com/state-bucket/workspaces/production/service.tfstate",
		},
		{
			name:    "GCS default workspace",
			backend: "gcs",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			wantRequest: "storage.googleapis.com/state-bucket/environment/service/default.tfstate",
		},
		{
			name:      "GCS named workspace",
			backend:   "gcs",
			workspace: "production",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			wantRequest: "storage.googleapis.com/state-bucket/environment/service/production.tfstate",
		},
		{
			name:    "Azure default workspace",
			backend: "azurerm",
			backendConfig: fmt.Sprintf(`
        access_key           = %q
        container_name       = "state"
        key                  = "service.tfstate"
        storage_account_name = "stateaccount"`, accessKey),
			wantRequest: "stateaccount.blob.core.windows.net/state/service.tfstate",
			enableAzure: true,
		},
		{
			name:      "Azure named workspace",
			backend:   "azurerm",
			workspace: "production",
			backendConfig: fmt.Sprintf(`
        access_key           = %q
        container_name       = "state"
        key                  = "service.tfstate"
        storage_account_name = "stateaccount"`, accessKey),
			wantRequest: "stateaccount.blob.core.windows.net/state/service.tfstateenv:production",
			enableAzure: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))

			env := map[string]string{}
			if testCase.backend == "s3" {
				env["AWS_ACCESS_KEY_ID"] = "test-access-key"
				env["AWS_SECRET_ACCESS_KEY"] = "test-secret-key"
			}

			if testCase.workspace != "" {
				env["TF_WORKSPACE"] = testCase.workspace
			}

			cfg, err := parseDependencyStateFixture(
				t,
				recorder,
				testCase.backend,
				testCase.backendConfig,
				env,
				testCase.enableAzure,
				"",
			)
			require.NoError(t, err)
			assert.Equal(t, "from-direct", cfg.Inputs["result"])
			assert.Empty(t, recorder.invocations(), "a direct state read must not invoke OpenTofu")
			assert.Equal(t, 1, recorder.closeCount(), "the state response body must be closed")

			requestPaths := recorder.requestPaths()
			require.Len(t, requestPaths, 1)
			assert.Equal(t, testCase.wantRequest, requestPaths[0])
		})
	}
}

func TestDependencyStateUnsupportedConfigFallsBackToNativeOutput(t *testing.T) {
	t.Parallel()

	accessKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	testCases := []struct {
		env           map[string]string
		name          string
		backend       string
		backendConfig string
		enableAzure   bool
	}{
		{
			name:    "S3 invalid workspace prefix",
			backend: "s3",
			backendConfig: `
        bucket               = "state-bucket"
        key                  = "service.tfstate"
        region               = "us-east-1"
        workspace_key_prefix = "/workspaces"`,
		},
		{
			name:    "GCS custom endpoint",
			backend: "gcs",
			backendConfig: `
        access_token            = "test-token"
        bucket                  = "state-bucket"
        prefix                  = "environment/service"
        storage_custom_endpoint = "https://storage.example.com"`,
		},
		{
			name:    "GCS executable environment set",
			backend: "gcs",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			env: map[string]string{"GOOGLE_EXTERNAL_ACCOUNT_ALLOW_EXECUTABLES": "1"},
		},
		{
			name:    "GCS invalid workspace",
			backend: "gcs",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			env: map[string]string{"TF_WORKSPACE": "invalid/workspace"},
		},
		{
			name:    "GCS dot workspace",
			backend: "gcs",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			env: map[string]string{"TF_WORKSPACE": "."},
		},
		{
			name:    "GCS parent-directory workspace",
			backend: "gcs",
			backendConfig: `
        access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
			env: map[string]string{"TF_WORKSPACE": ".."},
		},
		{
			name:    "Azure metadata host",
			backend: "azurerm",
			backendConfig: fmt.Sprintf(`
        access_key           = %q
        container_name       = "state"
        key                  = "service.tfstate"
        metadata_host        = "https://metadata.example.com"
        storage_account_name = "stateaccount"`, accessKey),
			enableAzure: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("unexpected-direct"))

			env := testCase.env
			if env == nil {
				env = map[string]string{}
			}

			ctx, pctx, configPath := prepareDependencyStateFixture(
				t,
				recorder,
				testCase.backend,
				testCase.backendConfig,
				env,
				testCase.enableAzure,
				"",
			)

			cfg, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), configPath, nil)
			require.NoError(t, err)
			assert.Equal(t, "from-native-output", cfg.Inputs["result"])
			assert.Empty(t, recorder.requestPaths(), "an unsupported config must not use the direct state client")

			invocations := recorder.invocations()
			require.NotEmpty(t, invocations)
			assert.True(t, slices.ContainsFunc(invocations, func(inv vexec.Invocation) bool {
				return slices.Contains(inv.Args, "output") && slices.Contains(inv.Args, "-json")
			}))
		})
	}
}

func TestDependencyStateGCSCustomerEncryptionKey(t *testing.T) {
	t.Parallel()

	encryptionKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("encrypted"))
	cfg, err := parseDependencyStateFixture(
		t,
		recorder,
		"gcs",
		fmt.Sprintf(`access_token  = "test-token"
        bucket        = "state-bucket"
        encryption_key = %q`, encryptionKey),
		map[string]string{},
		false,
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, "encrypted", cfg.Inputs["result"])

	headers := recorder.requestHeaders()
	require.Len(t, headers, 1)
	assert.Equal(t, "AES256", headers[0].Get("X-Goog-Encryption-Algorithm"))
	assert.Equal(t, encryptionKey, headers[0].Get("X-Goog-Encryption-Key"))
}

func TestDependencyStateMalformedDirectStateReturnsTypedError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cause error
		name  string
		state string
	}{
		{
			cause: io.ErrUnexpectedEOF,
			name:  "truncated JSON",
			state: `{"version":`,
		},
		{
			name:  "non-object JSON",
			state: `[]`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, []byte(testCase.state))
			_, err := parseDependencyStateFixture(
				t,
				recorder,
				"gcs",
				`access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
				map[string]string{},
				false,
				"",
			)

			require.Error(t, err)

			var parseErr config.DependencyStateParseError
			require.ErrorAs(t, err, &parseErr)
			assert.Equal(t, "gs://state-bucket/environment/service/default.tfstate", parseErr.Location)
			require.Error(t, parseErr.Err)

			if testCase.cause != nil {
				require.ErrorIs(t, err, testCase.cause)
			}

			assert.Empty(t, recorder.invocations(), "malformed direct state must not be retried through OpenTofu")
			assert.Equal(t, 1, recorder.closeCount(), "a malformed state response body must be closed")
		})
	}
}

func TestDependencyStateStopsReadingAfterOutputs(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusOK, nil)
	recorder.respond = func(*http.Request) *http.Response {
		resp := vhttp.Respond(http.StatusOK, nil, nil)
		resp.Body = io.NopCloser(&readerWithTerminalError{
			data: []byte(
				`{"version":4,"outputs":{"producer_value":{"sensitive":false,"type":"string","value":"from-direct"}},"resources":`,
			),
			err: io.ErrClosedPipe,
		})

		return resp
	}

	cfg, err := parseDependencyStateFixture(
		t,
		recorder,
		"s3",
		`bucket                      = "state-bucket"
        endpoint                    = "https://s3.example.com"
        force_path_style            = true
        key                         = "service.tfstate"
        region                      = "us-east-1"
        skip_credentials_validation = true`,
		map[string]string{
			"AWS_ACCESS_KEY_ID":     "test-access-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret-key",
		},
		false,
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, "from-direct", cfg.Inputs["result"])
	assert.Empty(t, recorder.invocations(), "an early direct read must not invoke OpenTofu")
	assert.Equal(t, 1, recorder.closeCount(), "an early direct read must close the state body")
}

func TestDependencyStateReadsOutputsAfterResources(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusOK, []byte(
		`{"version":4,"resources":[{"mode":"managed","type":"test_resource"}],"outputs":{"producer_value":{"sensitive":false,"type":"string","value":"from-direct"}}}`,
	))
	cfg, err := parseDependencyStateFixture(
		t,
		recorder,
		"gcs",
		`access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
		map[string]string{},
		false,
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, "from-direct", cfg.Inputs["result"])
	assert.Empty(t, recorder.invocations(), "a direct state read must not invoke OpenTofu")
	assert.Equal(t, 1, recorder.closeCount(), "a direct state read must close the state body")
}

func TestDependencyStateTransportFailureIsNotReportedAsMalformedState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		data string
	}{
		{
			name: "while decoding outputs",
			data: `{"version":4,"outputs":`,
		},
		{
			name: "between top-level fields",
			data: `{"version":4`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, nil)
			recorder.respond = func(*http.Request) *http.Response {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = io.NopCloser(&readerWithTerminalError{
					data: []byte(testCase.data),
					err:  io.ErrClosedPipe,
				})

				return resp
			}

			_, err := parseDependencyStateFixture(
				t,
				recorder,
				"s3",
				`bucket                      = "state-bucket"
        endpoint                    = "https://s3.example.com"
        force_path_style            = true
        key                         = "service.tfstate"
        region                      = "us-east-1"
        skip_credentials_validation = true`,
				map[string]string{
					"AWS_ACCESS_KEY_ID":     "test-access-key",
					"AWS_SECRET_ACCESS_KEY": "test-secret-key",
				},
				false,
				"",
			)

			var readErr config.DependencyStateReadError
			require.ErrorAs(t, err, &readErr)
			assert.Equal(t, "s3://state-bucket/service.tfstate", readErr.Location)
			require.ErrorIs(t, err, io.ErrClosedPipe)
			assert.Empty(t, recorder.invocations(), "a failed direct read must not invoke OpenTofu")
			assert.Equal(t, 1, recorder.closeCount(), "a failed direct read must close the state body")
		})
	}
}

func TestDependencyStateMissingDirectStateUsesMockOutputs(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusNotFound, []byte(`{"error":{"code":404,"message":"missing"}}`))
	cfg, err := parseDependencyStateFixture(
		t,
		recorder,
		"gcs",
		`access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
		map[string]string{},
		false,
		`mock_outputs = { producer_value = "from-mock" }`,
	)

	require.NoError(t, err)
	assert.Equal(t, "from-mock", cfg.Inputs["result"])
	assert.Empty(t, recorder.invocations(), "missing direct state must use mocks without invoking OpenTofu")
	assert.Equal(t, 1, recorder.closeCount(), "a missing-state response body must be closed")
}

func TestDependencyStateAzureClientSetupFailureDoesNotUseMocks(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusOK, nil)
	recorder.respond = func(req *http.Request) *http.Response {
		headers := http.Header{"Content-Type": []string{"application/json"}}

		switch {
		case strings.HasSuffix(req.URL.Path, "/discovery/instance"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"tenant_discovery_endpoint":"https://login.microsoftonline.com/tenant/v2.0/.well-known/openid-configuration","metadata":[{"preferred_network":"login.microsoftonline.com","preferred_cache":"login.microsoftonline.com","aliases":["login.microsoftonline.com"]}]}`,
			), headers)
		case strings.HasSuffix(req.URL.Path, "/openid-configuration"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"token_endpoint":"https://login.microsoftonline.com/tenant/oauth2/v2.0/token","issuer":"https://login.microsoftonline.com/tenant/v2.0","authorization_endpoint":"https://login.microsoftonline.com/tenant/oauth2/v2.0/authorize"}`,
			), headers)
		case strings.HasSuffix(req.URL.Path, "/token"):
			return vhttp.Respond(http.StatusOK, []byte(
				`{"token_type":"Bearer","expires_in":3600,"access_token":"test-token"}`,
			), headers)
		}

		headers.Set("X-Ms-Error-Code", "ResourceGroupNotFound")

		return vhttp.Respond(http.StatusNotFound, []byte(
			`{"error":{"code":"ResourceGroupNotFound","message":"missing resource group"}}`,
		), headers)
	}

	_, err := parseDependencyStateFixture(
		t,
		recorder,
		"azurerm",
		`client_id            = "client"
        client_secret        = "secret"
        container_name       = "state"
        key                  = "service.tfstate"
        resource_group_name  = "missing-group"
        storage_account_name = "stateaccount"
        subscription_id      = "subscription"
        tenant_id            = "tenant"`,
		map[string]string{},
		true,
		`mock_outputs = { producer_value = "from-mock" }`,
	)

	require.Error(t, err)

	var setupErr *azurermbackend.StateClientSetupError
	require.ErrorAs(t, err, &setupErr)

	var coordinateErr *azurermbackend.StateClientCoordinatesError
	require.ErrorAs(t, err, &coordinateErr)
	assert.Empty(t, recorder.invocations(), "a client setup failure must not invoke OpenTofu")
	requests := recorder.requestPaths()
	assert.True(t, slices.ContainsFunc(requests, func(request string) bool {
		return strings.Contains(request, "/listKeys")
	}), "the direct read must reach the ARM key lookup")
	assert.False(t, slices.ContainsFunc(requests, func(request string) bool {
		return strings.Contains(request, ".blob.core.windows.net")
	}), "a failed ARM lookup must not reach blob storage")
}

func TestDependencyStateEncryptedDirectStateFallsBackToNativeOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		state string
	}{
		{
			name:  "envelope first",
			state: `{"encrypted_data":"Y2lwaGVydGV4dA==","encryption_version":"v0","meta":{"key_provider.pbkdf2.default":{"salt":"c2FsdA=="}}}`,
		},
		{
			name:  "envelope after metadata",
			state: `{"serial":3,"lineage":"6d4c9f18","meta":{"key_provider.pbkdf2.default":{"salt":"c2FsdA=="}},"encryption_version":"v0","encrypted_data":"Y2lwaGVydGV4dA=="}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, []byte(testCase.state))
			cfg, err := parseDependencyStateFixture(
				t,
				recorder,
				"gcs",
				`access_token = "test-token"
        bucket       = "state-bucket"
        prefix       = "environment/service"`,
				map[string]string{},
				false,
				"",
			)

			require.NoError(t, err)
			assert.Equal(t, "from-native-output", cfg.Inputs["result"])
			assert.NotEmpty(t, recorder.invocations(), "encrypted direct state must fall back to OpenTofu")
			assert.Equal(t, 1, recorder.closeCount(), "an encrypted state response body must be closed")
		})
	}
}

type dependencyStateRecorder struct {
	respond    func(*http.Request) *http.Response
	httpBody   []byte
	requests   []string
	headers    []http.Header
	execs      []vexec.Invocation
	mu         sync.Mutex
	httpStatus int
	closes     int
}

func newDependencyStateRecorder(t *testing.T, status int, body []byte) *dependencyStateRecorder {
	t.Helper()

	return &dependencyStateRecorder{httpStatus: status, httpBody: body}
}

func (r *dependencyStateRecorder) httpClient() vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		r.mu.Lock()
		r.requests = append(r.requests, req.URL.Host+req.URL.Path)
		r.headers = append(r.headers, req.Header.Clone())
		r.mu.Unlock()

		resp := vhttp.Respond(r.httpStatus, r.httpBody, nil)
		if r.respond != nil {
			resp = r.respond(req)
		}

		resp.Body = &trackingReadCloser{
			ReadCloser: resp.Body,
			onClose: func() {
				r.mu.Lock()
				r.closes++
				r.mu.Unlock()
			},
		}

		return resp, nil
	})
}

func (r *dependencyStateRecorder) exec() vexec.Exec {
	return vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		r.mu.Lock()
		r.execs = append(r.execs, inv)
		r.mu.Unlock()

		if slices.Contains(inv.Args, "output") {
			return vexec.Result{Stdout: terraformOutput("from-native-output")}
		}

		return vexec.Result{}
	})
}

func (r *dependencyStateRecorder) requestPaths() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.requests)
}

func (r *dependencyStateRecorder) invocations() []vexec.Invocation {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.execs)
}

func (r *dependencyStateRecorder) requestHeaders() []http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.headers)
}

func (r *dependencyStateRecorder) closeCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.closes
}

type trackingReadCloser struct {
	io.ReadCloser
	onClose func()
}

func (r *trackingReadCloser) Close() error {
	r.onClose()

	return r.ReadCloser.Close()
}

type readerWithTerminalError struct {
	err  error
	data []byte
}

func (r *readerWithTerminalError) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}

	n := copy(p, r.data)
	r.data = r.data[n:]

	return n, nil
}

func parseDependencyStateFixture(
	t *testing.T,
	recorder *dependencyStateRecorder,
	backend string,
	backendConfig string,
	env map[string]string,
	enableAzure bool,
	dependencyExtra string,
) (*config.TerragruntConfig, error) {
	t.Helper()

	ctx, pctx, configPath := prepareDependencyStateFixture(
		t,
		recorder,
		backend,
		backendConfig,
		env,
		enableAzure,
		dependencyExtra,
	)

	return config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), configPath, nil)
}

func prepareDependencyStateFixture(
	t *testing.T,
	recorder *dependencyStateRecorder,
	backend string,
	backendConfig string,
	env map[string]string,
	enableAzure bool,
	dependencyExtra string,
) (context.Context, *config.ParsingContext, string) {
	t.Helper()

	const (
		consumerPath = "/repo/consumer/terragrunt.hcl"
		producerPath = "/repo/producer/terragrunt.hcl"
	)

	effectiveEnv := maps.Clone(env)
	if effectiveEnv == nil {
		effectiveEnv = map[string]string{}
	}

	v := venvtest.New().
		WithEnv(effectiveEnv).
		WithExec(recorder.exec()).
		WithHTTP(recorder.httpClient())

	for _, dir := range []string{filepath.Dir(consumerPath), filepath.Dir(producerPath)} {
		require.NoError(t, v.FS.MkdirAll(dir, 0o700))
	}

	producer := fmt.Sprintf(`remote_state {
  backend = %q
  config = {
%s
  }
}
`, backend, backendConfig)
	consumer := fmt.Sprintf(`dependency "producer" {
  config_path = "../producer"
  %s
}

inputs = {
  result = dependency.producer.outputs.producer_value
}
`, dependencyExtra)

	require.NoError(t, vfs.WriteFile(v.FS, producerPath, []byte(producer), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, consumerPath, []byte(consumer), 0o600))

	ctx, pctx := newTestParsingContext(t, v, consumerPath)
	ctx = config.WithConfigValues(ctx)
	pctx.OriginalTerragruntConfigPath = consumerPath
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.DependencyFetchOutputFromState))

	if enableAzure {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.AzureBackend))
	}

	return ctx, pctx, consumerPath
}

func terraformState(value string) []byte {
	return []byte(fmt.Sprintf(
		`{"version":4,"outputs":{"producer_value":{"sensitive":false,"type":"string","value":%q}}}`,
		value,
	))
}

func terraformOutput(value string) []byte {
	return []byte(fmt.Sprintf(
		`{"producer_value":{"sensitive":false,"type":"string","value":%q}}`,
		value,
	))
}
