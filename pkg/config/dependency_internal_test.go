package config

import (
	"path/filepath"
	"testing"

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
		want   string // "" means the dep is an explicit unit config, not a stack candidate
	}{
		{"stackFileDirectly", filepath.Join(tmpDir, DefaultStackFile), wantStack, wantStack},
		{"explicitTerragruntHCL", filepath.Join(tmpDir, DefaultTerragruntConfigPath), filepath.Join(tmpDir, DefaultTerragruntConfigPath), ""},
		{"explicitTerragruntJSON", filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), ""},
		{"bareDirectory", tmpDir, filepath.Join(tmpDir, DefaultTerragruntConfigPath), wantStack},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, resolveStackFilePath(tc.raw, tc.target))
		})
	}
}

// FuzzResolveStackFilePath verifies the path-rewrite helper never panics and every handled candidate ends in DefaultStackFile. raw is the user-authored dependency.config_path; target is the post-cleanup absolute path the resolver actually inspects — the two diverge in production (bare-directory raw + cleaned target with appended DefaultTerragruntConfigPath), so fuzz them independently.
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
		got := resolveStackFilePath(raw, target)
		if got != "" {
			require.Equal(t, DefaultStackFile, filepath.Base(got), "resolveStackFilePath must return either \"\" or a path whose base is %s (raw=%q target=%q got=%q)", DefaultStackFile, raw, target, got)
		}
	})
}
