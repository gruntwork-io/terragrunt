package venv_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/stretchr/testify/assert"
)

func TestParseEnviron(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want    map[string]string
		name    string
		environ []string
	}{
		{
			name:    "standard entries",
			environ: []string{"FOO=bar", "BAZ=qux"},
			want:    map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:    "value contains equals",
			environ: []string{"URL=https://example.com/?a=b"},
			want:    map[string]string{"URL": "https://example.com/?a=b"},
		},
		{
			name:    "empty value",
			environ: []string{"EMPTY="},
			want:    map[string]string{"EMPTY": ""},
		},
		{
			name:    "entry without separator is dropped",
			environ: []string{"NOSEP"},
			want:    map[string]string{},
		},
		{
			name:    "windows per-drive key keeps leading equals",
			environ: []string{`=C:=C:\Users\alice`},
			want:    map[string]string{`=C:`: `C:\Users\alice`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, venv.ParseEnviron(tc.environ))
		})
	}
}

func TestWithEnvClonedIsolatesMutations(t *testing.T) {
	t.Parallel()

	v := venv.Venv{Env: map[string]string{"FOO": "bar"}}

	clone := v.WithEnvCloned()
	clone.Env["AWS_ACCESS_KEY_ID"] = "leaked"
	clone.Env["FOO"] = "changed"

	assert.Equal(t, map[string]string{"FOO": "bar"}, v.Env)

	v.Env["BAZ"] = "qux"

	assert.NotContains(t, clone.Env, "BAZ")
}
