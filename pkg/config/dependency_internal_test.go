package config

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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
		{
			"explicitTerragruntHCL",
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			"",
			false,
		},
		{
			"explicitTerragruntJSON",
			filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath),
			filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath),
			"",
			false,
		},
		{
			"bareDirectory",
			tmpDir,
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			wantStack,
			true,
		},
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
			require.Empty(
				t,
				got,
				"resolveStackFilePath must return empty string when ok=false (raw=%q target=%q got=%q)",
				raw,
				target,
				got,
			)

			return
		}

		require.Equal(
			t,
			DefaultStackFile,
			filepath.Base(got),
			"resolveStackFilePath must return a path whose base is %s when ok=true (raw=%q target=%q got=%q)",
			DefaultStackFile,
			raw,
			target,
			got,
		)
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
					{
						Name:     "secrets",
						Commands: []string{"output", "plan"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "secret"},
		},
		{
			name:    "skips env vars when commands exclude output",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: []string{"apply", "plan"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
				},
			},
			want: map[string]string{},
		},
		{
			name:    "skips env vars when commands list is empty",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: nil,
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
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
			name:    "later block wins on overlapping keys",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "first",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"KEY": "first"}),
					},
					{
						Name:     "second",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"KEY": "second"}),
					},
				},
			},
			want: map[string]string{"KEY": "second"},
		},
		{
			name:    "extra args env vars override existing env",
			initial: map[string]string{"TF_VAR_passphrase": "old"},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "new"}),
					},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "new"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pctx := &ParsingContext{Venv: venvtest.NewOSWithEmptyEnv().WithEnv(tc.initial)}
			applyExtraArgsEnvVarsForOutput(pctx, tc.terraform)
			assert.Equal(t, tc.want, pctx.Venv.Env)
		})
	}
}

// TestGCSCredentialFileDirectStateReadSupported covers every credential file shape the gate decides on, including the external_account sources of issue #6810.
func TestGCSCredentialFileDirectStateReadSupported(t *testing.T) {
	t.Parallel()

	const credentialPath = "/config/credentials.json"

	testCases := []struct {
		name     string
		contents string
		want     bool
	}{
		{name: "service account", contents: `{"type":"service_account"}`, want: true},
		{name: "authorized user", contents: `{"type":"authorized_user"}`, want: true},
		{
			name:     "external account with a file source",
			contents: `{"type":"external_account","credential_source":{"file":"/var/run/token"}}`,
			want:     true,
		},
		{
			name:     "external account with a relative file source",
			contents: `{"type":"external_account","credential_source":{"file":"token.json"}}`,
		},
		{
			name:     "external account with a tilde file source",
			contents: `{"type":"external_account","credential_source":{"file":"~/token"}}`,
		},
		{
			name:     "external account with a kubernetes token file and format",
			contents: `{"type":"external_account","credential_source":{"file":"/var/run/service-account/token","format":{"type":"text"}}}`,
			want:     true,
		},
		{
			name:     "external account with a url source and headers",
			contents: `{"type":"external_account","credential_source":{"url":"https://sts.example.com/token","headers":{"Authorization":"Bearer x"},"format":{"type":"json","subject_token_field_name":"value"}}}`,
			want:     true,
		},
		{
			name:     "external account with a url source",
			contents: `{"type":"external_account","credential_source":{"url":"https://sts.example.com/token"}}`,
			want:     true,
		},
		{
			name:     "external account with an aws source",
			contents: `{"type":"external_account","credential_source":{"environment_id":"aws1","region_url":"http://169.254.169.254/latest/meta-data/placement/availability-zone","url":"http://169.254.169.254/latest/meta-data/iam/security-credentials","regional_cred_verification_url":"https://sts.{region}.amazonaws.com"}}`,
		},
		{
			name:     "external account with a certificate source",
			contents: `{"type":"external_account","credential_source":{"certificate":{"use_default_certificate_config":true}}}`,
		},
		{
			name:     "external account with an executable alongside a url",
			contents: `{"type":"external_account","credential_source":{"url":"https://sts.example.com/token","executable":{"command":"/usr/bin/token"}}}`,
		},
		{
			name:     "external account with an executable source",
			contents: `{"type":"external_account","credential_source":{"executable":{"command":"/usr/bin/token"}}}`,
		},
		{
			name:     "external account with no source",
			contents: `{"type":"external_account"}`,
		},
		{
			name:     "external account with an unrecognized source",
			contents: `{"type":"external_account","credential_source":{"future":"kind"}}`,
		},
		{
			name:     "external account with a null credential source",
			contents: `{"type":"external_account","credential_source":null}`,
		},
		{name: "external account authorized user", contents: `{"type":"external_account_authorized_user"}`},
		{name: "impersonated service account", contents: `{"type":"impersonated_service_account"}`},
		{name: "missing type", contents: `{}`},
		{name: "empty object", contents: `{"type":""}`},
		{name: "malformed json", contents: `{"type":`},
		{name: "trailing content", contents: `{"type":"service_account"}{"extra":true}`},
		{name: "not an object", contents: `"service_account"`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New()
			require.NoError(t, vfs.WriteFile(v.FS, credentialPath, []byte(testCase.contents), 0o600))

			pctx := &ParsingContext{Venv: v}

			assert.Equal(t, testCase.want, gcsCredentialFileDirectStateReadSupported(pctx, credentialPath))
		})
	}
}

// TestGCSCredentialFileDirectStateReadSupportedMissingFile pins that an unreadable credential file falls back instead of reading state as the wrong identity.
func TestGCSCredentialFileDirectStateReadSupportedMissingFile(t *testing.T) {
	t.Parallel()

	pctx := &ParsingContext{Venv: venvtest.New()}

	assert.True(t, gcsCredentialFileDirectStateReadSupported(pctx, ""), "an unset credential path is not a divergence")
	assert.False(t, gcsCredentialFileDirectStateReadSupported(pctx, "/config/absent.json"))
}
