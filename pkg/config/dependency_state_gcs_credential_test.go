package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gcsStatePath = "storage.googleapis.com/state-bucket/environment/service/default.tfstate"

	// gcsImpersonatedTokenResponse is the IAM Credentials reply that mints impersonated-token.
	gcsImpersonatedTokenResponse = `{"accessToken":"impersonated-token","expireTime":"2099-01-01T00:00:00Z"}`
)

var gcsCredentialPath = venvtest.Root("/credentials/service-account.json")

var errGCSCredentialClose = errors.New("closing GCS credential file")

func TestDependencyStateEligibilityStreamsGCSCredentialFile(t *testing.T) {
	t.Parallel()

	serviceAccount := testGCSServiceAccountJSON(t)

	t.Run("valid service account remains direct", func(t *testing.T) {
		t.Parallel()

		cfg, recorder, fsys := parseGCSCredentialEligibilityFixture(t, serviceAccount, nil)

		assert.Equal(t, "from-direct", cfg.Inputs["result"])
		assert.Empty(t, recorder.invocations())
		assert.Contains(t, recorder.requestPaths(), gcsStatePath)
		assert.Equal(t, int32(2), fsys.credentialOpens.Load())
		assert.Greater(t, fsys.reads.Load(), int32(1))
		assert.Equal(t, int32(1), fsys.closes.Load())
	})

	t.Run("trailing JSON falls back", func(t *testing.T) {
		t.Parallel()

		cfg, recorder, fsys := parseGCSCredentialEligibilityFixture(
			t,
			`{"type":"service_account"}{"unexpected":true}`,
			nil,
		)

		assert.Equal(t, "from-native-output", cfg.Inputs["result"])
		assert.Empty(t, recorder.requestPaths())
		assertEligibilityOutputInvocation(t, recorder.invocations())
		assert.Equal(t, int32(1), fsys.credentialOpens.Load())
		assert.Equal(t, int32(1), fsys.closes.Load())
	})

	t.Run("close failure falls back", func(t *testing.T) {
		t.Parallel()

		cfg, recorder, fsys := parseGCSCredentialEligibilityFixture(
			t,
			`{"type":"service_account"}`,
			errGCSCredentialClose,
		)

		assert.Equal(t, "from-native-output", cfg.Inputs["result"])
		assert.Empty(t, recorder.requestPaths())
		assertEligibilityOutputInvocation(t, recorder.invocations())
		assert.Equal(t, int32(1), fsys.credentialOpens.Load())
		assert.Equal(t, int32(1), fsys.closes.Load())
	})
}

// TestDependencyStateEligibilityAcceptsExternalAccountCredentials covers issue #6810: a Workload Identity Federation credentials file must keep direct state reads enabled.
func TestDependencyStateEligibilityAcceptsExternalAccountCredentials(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		source         string
		viaEnvironment bool
		withTokenFile  bool
		wantDirect     bool
	}{
		{
			name:           "credentials exported through GOOGLE_APPLICATION_CREDENTIALS remain direct",
			withTokenFile:  true,
			viaEnvironment: true,
			wantDirect:     true,
		},
		{
			name:          "file sourced subject token remains direct",
			withTokenFile: true,
			wantDirect:    true,
		},
		{
			name:       "url sourced subject token remains direct",
			source:     `{"url":"https://sts.example.com/subject-token"}`,
			wantDirect: true,
		},
		{
			// An AWS source reads its credentials from the process environment.
			name:   "aws sourced subject token falls back",
			source: `{"environment_id":"aws1"}`,
		},
		{
			// An executable source would run with the Terragrunt process environment.
			name:   "executable sourced subject token falls back",
			source: `{"executable":{"command":"/usr/bin/get-token","timeout_millis":5000}}`,
		},
		{
			name:   "executable alongside a file source still falls back",
			source: `{"file":"/credentials/subject-token","executable":{"command":"/usr/bin/get-token"}}`,
		},
		{
			name:   "empty credential source falls back",
			source: `{}`,
		},
		{
			name:   "unrecognized credential source falls back",
			source: `{"future_source":{"kind":"unknown"}}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			source := testCase.source
			if testCase.withTokenFile {
				source = fmt.Sprintf(`{"file":%q}`, writeSubjectTokenFile(t))
			}

			cfg, recorder := parseGCSExternalAccountFixture(t, externalAccountCredentials(source), testCase.viaEnvironment)

			if !testCase.wantDirect {
				assert.Equal(t, "from-native-output", cfg.Inputs["result"])
				assert.Empty(t, recorder.requestPaths(), "a rejected credential must not reach the network")
				assertEligibilityOutputInvocation(t, recorder.invocations())

				return
			}

			assert.Equal(t, "from-direct", cfg.Inputs["result"])
			assert.Contains(t, recorder.requestPaths(), gcsStatePath)
			assert.Empty(t, recorder.invocations(), "a direct read must not run the native output command")

			paths := strings.Join(recorder.requestPaths(), " ")
			assert.Contains(t, paths, "sts.googleapis.com", "the subject token must be exchanged")
			assert.Contains(t, paths, "iamcredentials.googleapis.com", "the impersonation chain must be followed")
		})
	}
}

// TestDependencyStateEligibilityExternalAccountCredentialIOFailure pins that a credential file whose close fails keeps the unit on the native output path.
func TestDependencyStateEligibilityExternalAccountCredentialIOFailure(t *testing.T) {
	t.Parallel()

	cfg, recorder, fsys := parseGCSCredentialEligibilityFixture(
		t,
		externalAccountCredentials(`{"file":"/var/run/subject-token"}`),
		errGCSCredentialClose,
	)

	assert.Equal(t, "from-native-output", cfg.Inputs["result"])
	assert.Empty(t, recorder.requestPaths())
	assert.Equal(t, int32(1), fsys.closes.Load())
}

// TestDependencyStateEligibilityImpersonatesLikeNativeBackend pins that impersonation keeps direct state reads, resolving the service account, delegates, scope and source identity the way the native GCS backend does.
func TestDependencyStateEligibilityImpersonatesLikeNativeBackend(t *testing.T) {
	t.Parallel()

	serviceAccount := testGCSServiceAccountJSON(t)
	configuredAccount := map[string]string{"impersonate_service_account": `"config@example.com"`}
	readWriteScope := []string{"https://www.googleapis.com/auth/devstorage.read_write"}

	testCases := []struct {
		name              string
		backendConfig     map[string]string
		env               map[string]string
		files             map[string]string
		removeConfig      []string
		wantRequests      []string
		wantAuthorization []string
		wantScope         []string
		wantDelegates     []string
	}{
		{
			name:              "configured service account remains direct",
			backendConfig:     configuredAccount,
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name:              "GOOGLE_IMPERSONATE_SERVICE_ACCOUNT remains direct",
			env:               map[string]string{"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": "env@example.com"},
			wantRequests:      []string{gcsGenerateAccessTokenPath("env@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name: "GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT wins over GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
			env: map[string]string{
				"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT": "backend@example.com",
				"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT":         "env@example.com",
			},
			wantRequests:      []string{gcsGenerateAccessTokenPath("backend@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name:              "configured service account wins over the environment",
			backendConfig:     configuredAccount,
			env:               map[string]string{"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT": "backend@example.com"},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name:              "configured empty service account suppresses GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT",
			backendConfig:     map[string]string{"impersonate_service_account": `""`},
			env:               map[string]string{"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT": "backend@example.com"},
			wantRequests:      []string{gcsStatePath},
			wantAuthorization: []string{"Bearer test-token"},
		},
		{
			name:              "configured empty service account suppresses GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
			backendConfig:     map[string]string{"impersonate_service_account": `""`},
			env:               map[string]string{"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT": "env@example.com"},
			wantRequests:      []string{gcsStatePath},
			wantAuthorization: []string{"Bearer test-token"},
		},
		{
			name: "configured delegates are forwarded",
			backendConfig: map[string]string{
				"impersonate_service_account":           `"config@example.com"`,
				"impersonate_service_account_delegates": `["delegate@example.com"]`,
			},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
			wantDelegates:     []string{"projects/-/serviceAccounts/delegate@example.com"},
		},
		{
			name: "null delegates remain direct",
			backendConfig: map[string]string{
				"impersonate_service_account":           `"config@example.com"`,
				"impersonate_service_account_delegates": "null",
			},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name: "empty delegates remain direct",
			backendConfig: map[string]string{
				"impersonate_service_account":           `"config@example.com"`,
				"impersonate_service_account_delegates": "[]",
			},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			// Neither native backend reads delegates from the environment.
			name:          "delegates environment variables are ignored like the native backend",
			backendConfig: configuredAccount,
			env: map[string]string{
				"GOOGLE_BACKEND_IMPERSONATE_SERVICE_ACCOUNT_DELEGATES": "backend-delegate@example.com",
				"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT_DELEGATES":         "delegate@example.com",
			},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer test-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name: "credentials file signs the impersonation call",
			backendConfig: map[string]string{
				"credentials":                 strconv.Quote(gcsCredentialPath),
				"impersonate_service_account": `"config@example.com"`,
			},
			removeConfig: []string{"access_token"},
			files:        map[string]string{gcsCredentialPath: serviceAccount},
			wantRequests: []string{
				"oauth2.googleapis.com/token",
				gcsGenerateAccessTokenPath("config@example.com"),
				gcsStatePath,
			},
			wantAuthorization: []string{"", "Bearer service-account-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name:          "GOOGLE_APPLICATION_CREDENTIALS signs the impersonation call",
			backendConfig: configuredAccount,
			removeConfig:  []string{"access_token"},
			env:           map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": gcsCredentialPath},
			files:         map[string]string{gcsCredentialPath: serviceAccount},
			wantRequests: []string{
				"oauth2.googleapis.com/token",
				gcsGenerateAccessTokenPath("config@example.com"),
				gcsStatePath,
			},
			wantAuthorization: []string{"", "Bearer service-account-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
		{
			name:              "GOOGLE_OAUTH_ACCESS_TOKEN signs the impersonation call",
			backendConfig:     configuredAccount,
			removeConfig:      []string{"access_token"},
			env:               map[string]string{"GOOGLE_OAUTH_ACCESS_TOKEN": "environment-token"},
			wantRequests:      []string{gcsGenerateAccessTokenPath("config@example.com"), gcsStatePath},
			wantAuthorization: []string{"Bearer environment-token", "Bearer impersonated-token"},
			wantScope:         readWriteScope,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := dependencyStateEligibilityTestCase{
				backend:       "gcs",
				backendConfig: eligibilityConfig(eligibilityGCSConfig(), testCase.backendConfig, testCase.removeConfig...),
				env:           testCase.env,
				files:         testCase.files,
			}

			cfg, recorder, request := parseGCSImpersonationFixture(t, &fixture)

			require.NoError(t, request.decodeErr, "decoding generateAccessToken request")
			assert.Equal(t, "from-direct", cfg.Inputs["result"])
			assert.Empty(t, recorder.invocations(), "a direct read must not run the native output command")
			require.Equal(t, testCase.wantRequests, recorder.requestPaths())

			authorization := make([]string, 0, len(testCase.wantRequests))
			for _, header := range recorder.requestHeaders() {
				authorization = append(authorization, header.Get("Authorization"))
			}

			assert.Equal(t, testCase.wantAuthorization, authorization, "the source identity signs the IAM call and the impersonated token reads the state")
			assert.Equal(t, testCase.wantScope, request.Scope, "the native backend requests read_write")
			assert.Equal(t, testCase.wantDelegates, request.Delegates)
		})
	}
}

func parseGCSCredentialEligibilityFixture(
	t *testing.T,
	credentials string,
	closeErr error,
) (*config.TerragruntConfig, *dependencyStateRecorder, *gcsCredentialStreamFS) {
	t.Helper()

	fsys := &gcsCredentialStreamFS{
		FS:             vfs.NewMemMapFS(),
		credentialPath: gcsCredentialPath,
		closeErr:       closeErr,
	}
	recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))
	recorder.respond = func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "oauth2.googleapis.com":
			return vhttp.Respond(
				http.StatusOK,
				[]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`),
				nil,
			)
		default:
			return vhttp.Respond(http.StatusOK, terraformState("from-direct"), nil)
		}
	}

	testCase := dependencyStateEligibilityTestCase{
		backend: "gcs",
		backendConfig: eligibilityConfig(
			eligibilityGCSConfig(),
			map[string]string{"credentials": strconv.Quote(gcsCredentialPath)},
			"access_token",
		),
		files:      map[string]string{gcsCredentialPath: credentials},
		filesystem: fsys,
	}

	cfg, err := parseDependencyStateEligibilityFixture(t, recorder, &testCase)
	require.NoError(t, err)

	return cfg, recorder, fsys
}

func testGCSServiceAccountJSON(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "test-project",
		"client_email": "test@test-project.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})),
	})
	require.NoError(t, err)

	return string(payload)
}

// gcsCredentialStreamFS tracks the eligibility probe's first credential-file handle.
type gcsCredentialStreamFS struct {
	vfs.FS
	closeErr        error
	credentialPath  string
	credentialOpens atomic.Int32
	reads           atomic.Int32
	closes          atomic.Int32
}

func (fsys *gcsCredentialStreamFS) Open(name string) (vfs.File, error) {
	file, err := fsys.FS.Open(name)

	switch {
	case err != nil:
		return nil, err
	case name != fsys.credentialPath:
		return file, nil
	case fsys.credentialOpens.Add(1) == 1:
		return &gcsCredentialStreamFile{File: file, fsys: fsys}, nil
	default:
		return file, nil
	}
}

type gcsCredentialStreamFile struct {
	vfs.File
	fsys *gcsCredentialStreamFS
}

func (file *gcsCredentialStreamFile) Read(p []byte) (int, error) {
	const maxChunk = 3

	file.fsys.reads.Add(1)

	return file.File.Read(p[:min(len(p), maxChunk)])
}

func (file *gcsCredentialStreamFile) Close() error {
	file.fsys.closes.Add(1)

	return errors.Join(file.File.Close(), file.fsys.closeErr)
}

// externalAccountCredentials builds the credentials file google-github-actions/auth writes, including the impersonation chain it adds for a named service account.
func externalAccountCredentials(credentialSource string) string {
	return `{"type":"external_account",` +
		`"audience":"//iam.googleapis.com/projects/1/locations/global/workloadIdentityPools/p/providers/g",` +
		`"subject_token_type":"urn:ietf:params:oauth:token-type:jwt",` +
		`"token_url":"https://sts.googleapis.com/v1/token",` +
		`"service_account_impersonation_url":"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/gh@p.iam.gserviceaccount.com:generateAccessToken",` +
		`"credential_source":` + credentialSource + `}`
}

// writeSubjectTokenFile uses the real filesystem because the Google SDK opens the subject-token path itself rather than through the unit's vfs.
func writeSubjectTokenFile(t *testing.T) string {
	t.Helper()

	tokenPath := filepath.Join(t.TempDir(), "subject-token")
	require.NoError(t, os.WriteFile(tokenPath, []byte("subject-token-value"), 0o600))

	return tokenPath
}

func parseGCSExternalAccountFixture(
	t *testing.T,
	credentials string,
	viaEnvironment bool,
) (*config.TerragruntConfig, *dependencyStateRecorder) {
	t.Helper()

	recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))
	recorder.respond = func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "sts.googleapis.com", "oauth2.googleapis.com", "sts.example.com":
			return vhttp.Respond(
				http.StatusOK,
				[]byte(`{"access_token":"federated-token","token_type":"Bearer","expires_in":3600,"issued_token_type":"urn:ietf:params:oauth:token-type:access_token"}`),
				nil,
			)
		case "iamcredentials.googleapis.com":
			return vhttp.Respond(http.StatusOK, []byte(gcsImpersonatedTokenResponse), nil)
		case "storage.googleapis.com":
			return vhttp.Respond(http.StatusOK, terraformState("from-direct"), nil)
		default:
			assert.Fail(t, "unexpected request to "+req.URL.Host)

			return vhttp.Respond(http.StatusInternalServerError, nil, nil)
		}
	}

	testCase := dependencyStateEligibilityTestCase{
		backend: "gcs",
		backendConfig: eligibilityConfig(
			eligibilityGCSConfig(),
			map[string]string{"credentials": strconv.Quote(gcsCredentialPath)},
			"access_token",
		),
		files: map[string]string{gcsCredentialPath: credentials},
	}

	// The reporter's setup exports the credentials file instead of naming it in the backend block.
	if viaEnvironment {
		testCase.backendConfig = eligibilityConfig(eligibilityGCSConfig(), nil, "access_token", "credentials")
		testCase.env = map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": gcsCredentialPath}
	}

	cfg, err := parseDependencyStateEligibilityFixture(t, recorder, &testCase)
	require.NoError(t, err)

	return cfg, recorder
}

// gcsGenerateAccessTokenRequest is the IAM Credentials request body the impersonation chain sends.
type gcsGenerateAccessTokenRequest struct {
	decodeErr error
	Delegates []string `json:"delegates"`
	Scope     []string `json:"scope"`
}

// parseGCSImpersonationFixture parses a GCS dependency with token exchange, IAM Credentials and storage served in memory,
// returning the generateAccessToken request it captured.
func parseGCSImpersonationFixture(
	t *testing.T,
	testCase *dependencyStateEligibilityTestCase,
) (*config.TerragruntConfig, *dependencyStateRecorder, *gcsGenerateAccessTokenRequest) {
	t.Helper()

	request := &gcsGenerateAccessTokenRequest{}

	recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))
	recorder.respond = func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "oauth2.googleapis.com":
			return vhttp.Respond(
				http.StatusOK,
				[]byte(`{"access_token":"service-account-token","token_type":"Bearer","expires_in":3600}`),
				nil,
			)
		case "iamcredentials.googleapis.com":
			request.decodeErr = json.NewDecoder(req.Body).Decode(request)

			return vhttp.Respond(http.StatusOK, []byte(gcsImpersonatedTokenResponse), nil)
		case "storage.googleapis.com":
			return vhttp.Respond(http.StatusOK, terraformState("from-direct"), nil)
		default:
			assert.Fail(t, "unexpected request to "+req.URL.Host)

			return vhttp.Respond(http.StatusInternalServerError, nil, nil)
		}
	}

	cfg, err := parseDependencyStateEligibilityFixture(t, recorder, testCase)
	require.NoError(t, err)

	return cfg, recorder, request
}

// gcsGenerateAccessTokenPath is the IAM Credentials request path that impersonates principal.
func gcsGenerateAccessTokenPath(principal string) string {
	return "iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/" + principal + ":generateAccessToken"
}
