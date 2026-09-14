package test_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureAuthProviderRoleReuse = "fixtures/auth-provider-cmd/role-session-reuse"
	authProviderTestRoleARN          = "arn:aws:iam::123456789012:role/integration-test"
	authProviderBaseKeyID            = "AKIAINTEGRATIONBASE"
	authProviderMintedKeyID          = "ASIAINTEGRATIONMINTED"
)

var authProviderCredRe = regexp.MustCompile(`Credential=([A-Z0-9]+)/`)

// Pins end to end that no sts:AssumeRole request is ever signed by a session Terragrunt minted.
func TestAuthProviderRoleIsAssumedWithCallerIdentity(t *testing.T) {
	// t.Setenv cannot be combined with t.Parallel.
	sts := newSTSRecorder(t)

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	t.Setenv("TG_TEST_ROLE_ARN", authProviderTestRoleARN)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL())
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", authProviderBaseKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")

	helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --working-dir %s --auth-provider-cmd %s",
		rootPath, authCmd,
	))

	signers := sts.signers()
	require.NotEmpty(t, signers, "the auth provider must have driven at least one sts:AssumeRole")

	for i, signer := range signers {
		assert.Equalf(t, authProviderBaseKeyID, signer,
			"sts:AssumeRole call %d of %d was signed by %q rather than the caller's identity %q; "+
				"a session Terragrunt minted must never sign an assume-role request",
			i+1, len(signers), signer, authProviderBaseKeyID)
	}
}

// Pins that the assumed session is reused rather than re-assumed for every unit and dependency.
func TestAuthProviderRoleIsAssumedOnce(t *testing.T) {
	// t.Setenv cannot be combined with t.Parallel.
	sts := newSTSRecorder(t)

	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderRoleReuse)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderRoleReuse)
	rootPath := filepath.Join(tmpEnvPath, testFixtureAuthProviderRoleReuse)
	authCmd := filepath.Join(rootPath, "auth-provider.sh")

	t.Setenv("TG_TEST_ROLE_ARN", authProviderTestRoleARN)
	t.Setenv("AWS_ENDPOINT_URL_STS", sts.URL())
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", authProviderBaseKeyID)
	t.Setenv("AWS_SECRET_ACCESS_KEY", "base-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")

	helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt run --all plan --non-interactive --working-dir %s --auth-provider-cmd %s",
		rootPath, authCmd,
	))

	assert.LessOrEqualf(t, len(sts.signers()), 1,
		"the role should be assumed at most once per process, got %d sts:AssumeRole calls; "+
			"more than one means the credentials cache is not being hit",
		len(sts.signers()))
}

type stsRecorder struct {
	server *httptest.Server
	seen   []string
	mu     sync.Mutex
}

func (s *stsRecorder) URL() string {
	return s.server.URL
}

func (s *stsRecorder) signers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.seen...)
}

func newSTSRecorder(t *testing.T) *stsRecorder {
	t.Helper()

	rec := &stsRecorder{}

	body := `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
		`<AssumeRoleResult><Credentials>` +
		`<AccessKeyId>` + authProviderMintedKeyID + `</AccessKeyId>` +
		`<SecretAccessKey>minted-secret</SecretAccessKey>` +
		`<SessionToken>minted-token</SessionToken>` +
		`<Expiration>2030-12-31T23:59:59Z</Expiration>` +
		`</Credentials><AssumedRoleUser>` +
		`<AssumedRoleId>AROATEST:session</AssumedRoleId>` +
		`<Arn>` + authProviderTestRoleARN + `</Arn>` +
		`</AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`

	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		signer := "NONE"
		if m := authProviderCredRe.FindStringSubmatch(r.Header.Get("Authorization")); m != nil {
			signer = m[1]
		}

		rec.mu.Lock()
		rec.seen = append(rec.seen, signer)
		rec.mu.Unlock()

		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(rec.server.Close)

	return rec
}
