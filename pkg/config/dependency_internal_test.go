package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractFirstJSONObject verifies that we can isolate the first JSON object emitted by
// `tofu/terraform output -json` even when stdout is polluted with non-JSON content on either
// side of the JSON. See https://github.com/gruntwork-io/terragrunt/issues/6001 for the trailing-
// warning regression introduced by Terraform 1.15, and #2233 for the leading AWS CSM log line.
func TestExtractFirstJSONObject(t *testing.T) {
	t.Parallel()

	const validJSON = `{"foo":{"sensitive":false,"type":"string","value":"bar"}}`

	tcs := []struct {
		name      string
		input     string
		want      string
		wantExact bool // when true, compare bytes exactly instead of as JSON
		wantErr   bool
	}{
		{
			name:  "pure JSON is returned unchanged",
			input: validJSON,
			want:  validJSON,
		},
		{
			name:  "leading AWS CSM log line is stripped",
			input: "2023/05/04 20:22:43 Enabling CSM" + validJSON,
			want:  validJSON,
		},
		{
			name:  "leading ANSI-colored warning is stripped",
			input: "\x1b[33m\x1b[1mWarning:\x1b[0m Deprecated Parameter\n\n" + validJSON,
			want:  validJSON,
		},
		{
			name: "trailing Terraform 1.15 deprecation warning is ignored",
			input: validJSON + "\n\n" +
				"Warning: Deprecated Parameter\n\n" +
				`The parameter "dynamodb_table" is deprecated. Use parameter "use_lockfile" instead.` + "\n",
			want: validJSON,
		},
		{
			name: "leading and trailing pollution together",
			input: "2023/05/04 20:22:43 Enabling CSM" + validJSON +
				"\nWarning: Deprecated Parameter\n",
			want: validJSON,
		},
		{
			name:  "empty outputs object is preserved",
			input: "Warning: something\n{}\nWarning: trailing\n",
			want:  "{}",
		},
		{
			name:      "no JSON returns input unchanged so downstream surfaces the underlying error",
			input:     "no json here at all",
			want:      "no json here at all",
			wantExact: true,
		},
		{
			name:    "truncated JSON object surfaces a parse error",
			input:   `{"foo":`,
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractFirstJSONObject([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.wantExact {
				assert.Equal(t, tc.want, string(got))
				return
			}

			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

// TestResolveStackFilePath pins resolveStackFilePath across dependency-target shapes (direct stack file, explicit terragrunt config, bare directory).
func TestResolveStackFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	wantStack := filepath.Join(tmpDir, DefaultStackFile)

	cases := []struct {
		name   string
		raw    string
		target string
		want   string
		wantOK bool
	}{
		{"stackFileDirectly", filepath.Join(tmpDir, DefaultStackFile), wantStack, wantStack, true},
		{"explicitTerragruntHCL", filepath.Join(tmpDir, DefaultTerragruntConfigPath), filepath.Join(tmpDir, DefaultTerragruntConfigPath), "", false},
		{"explicitTerragruntJSON", filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), "", false},
		{"bareDirectory", tmpDir, filepath.Join(tmpDir, DefaultTerragruntConfigPath), wantStack, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := resolveStackFilePath(tc.raw, tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// FuzzResolveStackFilePath verifies the helper never panics and every handled candidate ends in DefaultStackFile.
func FuzzResolveStackFilePath(f *testing.F) {
	seeds := [][2]string{
		{"/abs/dir/" + DefaultStackFile, "/abs/dir/" + DefaultStackFile},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntConfigPath},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntJSONConfigPath},
		{"/abs/dir/" + DefaultTerragruntConfigPath, "/abs/dir/" + DefaultTerragruntConfigPath},
		{"relative/dir", "relative/dir/" + DefaultTerragruntConfigPath},
		{"", ""},
		{".", "./" + DefaultTerragruntConfigPath},
		{"/", "/" + DefaultTerragruntConfigPath},
		{"\x00", "\x00"},
		{"unicode/café", "unicode/café/" + DefaultTerragruntConfigPath},
	}

	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, raw, target string) {
		got, ok := resolveStackFilePath(raw, target)
		if !ok {
			require.Empty(t, got, "resolveStackFilePath must return empty string when ok=false (raw=%q target=%q got=%q)", raw, target, got)
			return
		}

		require.Equal(t, DefaultStackFile, filepath.Base(got), "resolveStackFilePath must return a path whose base is %s when ok=true (raw=%q target=%q got=%q)", DefaultStackFile, raw, target, got)
	})
}

func TestApplyExtraArgsEnvVarsForOutput(t *testing.T) {
	t.Parallel()

	envVars := func(m map[string]string) *map[string]string { return &m }

	tcs := []struct {
		initial   map[string]string
		terraform *TerraformConfig
		want      map[string]string
		name      string
	}{
		{
			name:    "applies env vars when commands include output",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"output", "plan"}, EnvVars: envVars(map[string]string{"TF_VAR_passphrase": "secret"})},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "secret"},
		},
		{
			name:    "skips env vars when commands exclude output",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"apply", "plan"}, EnvVars: envVars(map[string]string{"TF_VAR_passphrase": "secret"})},
				},
			},
			want: map[string]string{},
		},
		{
			name:    "skips env vars when commands list is empty",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: nil, EnvVars: envVars(map[string]string{"TF_VAR_passphrase": "secret"})},
				},
			},
			want: map[string]string{},
		},
		{
			name:      "nil terraform config is a no-op",
			initial:   map[string]string{"EXISTING": "value"},
			terraform: nil,
			want:      map[string]string{"EXISTING": "value"},
		},
		{
			name:      "terraform config without extra args is a no-op",
			initial:   map[string]string{"EXISTING": "value"},
			terraform: &TerraformConfig{},
			want:      map[string]string{"EXISTING": "value"},
		},
		{
			name:    "nil env vars is a no-op",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"output"}, EnvVars: nil},
				},
			},
			want: map[string]string{},
		},
		{
			name:    "nil env map is initialized before applying",
			initial: nil,
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"output"}, EnvVars: envVars(map[string]string{"KEY": "value"})},
				},
			},
			want: map[string]string{"KEY": "value"},
		},
		{
			name:    "later block wins on overlapping keys",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "first", Commands: []string{"output"}, EnvVars: envVars(map[string]string{"KEY": "first"})},
					{Name: "second", Commands: []string{"output"}, EnvVars: envVars(map[string]string{"KEY": "second"})},
				},
			},
			want: map[string]string{"KEY": "second"},
		},
		{
			name:    "extra args env vars override existing env",
			initial: map[string]string{"TF_VAR_passphrase": "old"},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"output"}, EnvVars: envVars(map[string]string{"TF_VAR_passphrase": "new"})},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "new"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pctx := &ParsingContext{Env: tc.initial}
			applyExtraArgsEnvVarsForOutput(pctx, tc.terraform)
			assert.Equal(t, tc.want, pctx.Env)
		})
	}
}

// TestGetTerragruntOutputJSONFromRemoteStateS3UsesDependencyIAMRole is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/4979.
//
// When the dependency-fetch-output-from-state experiment is enabled and a dependency lives in an
// AWS account that requires its own IAM role to read state, getTerragruntOutputJSONFromRemoteStateS3
// must assume the dependency's own role (passed explicitly as iamRoleOpts) rather than whatever role
// happens to be left on pctx.IAMRoleOptions (which resolveOutputJSON clears to the zero value whenever
// it differs from the original). Before the fix, the function read pctx.IAMRoleOptions directly, so a
// cross-account dependency would be fetched with no role assumption at all -- and an unrelated role left
// on pctx.IAMRoleOptions would also be used incorrectly when present.
//
// This test does not talk to real AWS: it points the AWS SDK's STS endpoint resolution
// (AWS_ENDPOINT_URL_STS) at a local httptest server and inspects which RoleArn was actually requested.
func TestGetTerragruntOutputJSONFromRemoteStateS3UsesDependencyIAMRole(t *testing.T) {
	// Not parallel: mutates process-wide AWS_ENDPOINT_URL_STS / AWS_ENDPOINT_URL_S3 env vars.

	var (
		gotRoleArnsMu sync.Mutex
		gotRoleArns   []string
	)

	stsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		values, _ := url.ParseQuery(string(body))

		gotRoleArnsMu.Lock()
		gotRoleArns = append(gotRoleArns, values.Get("RoleArn"))
		gotRoleArnsMu.Unlock()

		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<AssumeRoleResponse><AssumeRoleResult><Credentials>` +
			`<AccessKeyId>fake-access-key</AccessKeyId>` +
			`<SecretAccessKey>fake-secret-key</SecretAccessKey>` +
			`<SessionToken>fake-session-token</SessionToken>` +
			`<Expiration>2999-01-01T00:00:00Z</Expiration>` +
			`</Credentials></AssumeRoleResult></AssumeRoleResponse>`))
	}))
	defer stsServer.Close()

	// A fake S3 endpoint is also required so BuildS3Client's GetObject call fails fast against a
	// known server instead of reaching out to real AWS; the role-assumption check above already
	// happens before this point, so its response content doesn't matter for this test.
	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer s3Server.Close()

	t.Setenv("AWS_ENDPOINT_URL_STS", stsServer.URL)
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")
	t.Setenv("AWS_REGION", "us-east-1")

	const (
		callingUnitRoleARN = "arn:aws:iam::111111111111:role/calling-unit-role"
		dependencyRoleARN  = "arn:aws:iam::222222222222:role/dependency-role"
	)

	l := logger.CreateLogger()

	// Simulate resolveOutputJSON having cleared pctx.IAMRoleOptions to the zero value (as it does
	// whenever it differs from the original), while an unrelated role is still floating around to
	// prove the fetch does not fall back to it.
	pctx := &ParsingContext{
		Env:            map[string]string{},
		IAMRoleOptions: iam.RoleOptions{RoleARN: callingUnitRoleARN},
	}

	remoteState := remotestate.New(&remotestate.Config{
		BackendName: "s3",
		BackendConfig: map[string]any{
			"bucket": "dependency-bucket",
			"key":    "terraform.tfstate",
			"region": "us-east-1",
			"endpoints": map[string]any{
				"s3": s3Server.URL,
			},
			"s3_force_path_style":         true,
			"skip_credentials_validation": true,
		},
	})

	// This is the dependency's own merged IAM role options, as computed by
	// getTerragruntOutputJSONFromRemoteState via iam.MergeRoleOptions(iamRoleOpts, pctx.OriginalIAMRoleOptions).
	dependencyMergedIAM := iam.RoleOptions{RoleARN: dependencyRoleARN}

	_, err := getTerragruntOutputJSONFromRemoteStateS3(t.Context(), l, pctx, remoteState, dependencyMergedIAM)
	// The S3 fetch itself is expected to fail (fake server returns 404 with no valid S3 error body) --
	// only the role used for credential resolution is under test here.
	require.Error(t, err)

	gotRoleArnsMu.Lock()
	defer gotRoleArnsMu.Unlock()

	require.NotEmpty(t, gotRoleArns, "expected the S3 client build to assume a role via STS")
	for _, roleArn := range gotRoleArns {
		assert.Equal(t, dependencyRoleARN, roleArn,
			"S3 client must assume the dependency's own IAM role, not the calling unit's role")
		assert.NotContains(t, strings.ToLower(roleArn), strings.ToLower(callingUnitRoleARN))
	}
}
