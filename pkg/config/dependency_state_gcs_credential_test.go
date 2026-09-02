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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	gcsCredentialPath = "/credentials/service-account.json"
	gcsStatePath      = "storage.googleapis.com/state-bucket/environment/service/default.tfstate"
)

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
			return vhttp.Respond(
				http.StatusOK,
				[]byte(`{"accessToken":"impersonated-token","expireTime":"2099-01-01T00:00:00Z"}`),
				nil,
			)
		case "storage.googleapis.com":
			return vhttp.Respond(http.StatusOK, terraformState("from-direct"), nil)
		default:
			t.Errorf("unexpected request to %s", req.URL.Host)

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
