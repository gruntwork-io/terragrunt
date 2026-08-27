package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"strconv"
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
