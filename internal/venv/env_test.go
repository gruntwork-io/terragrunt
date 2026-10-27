package venv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/venv"
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
			name:    "empty entry is dropped",
			environ: []string{"", "FOO=bar"},
			want:    map[string]string{"FOO": "bar"},
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

// TestWithEnvRejectsNil pins the argument contract: WithEnv asserts a
// non-nil env instead of silently substituting an empty map, and
// WithEnvCloned asserts a non-nil receiver Env before cloning.
func TestWithEnvRejectsNil(t *testing.T) {
	t.Parallel()

	assert.PanicsWithValue(t, venv.ErrVenvEnvNil, func() {
		(&venv.Venv{}).WithEnv(nil)
	})

	assert.PanicsWithValue(t, venv.ErrVenvEnvUnset, func() {
		(&venv.Venv{}).WithEnvCloned()
	})
}

func TestWithEnvClonedIsolatesMutations(t *testing.T) {
	t.Parallel()

	v := &venv.Venv{
		Env: map[string]string{"FOO": "bar"},
	}

	clone := v.WithEnvCloned()
	clone.Env["AWS_ACCESS_KEY_ID"] = "leaked"
	clone.Env["FOO"] = "changed"

	assert.Equal(t, map[string]string{"FOO": "bar"}, v.Env)

	v.Env["BAZ"] = "qux"

	assert.NotContains(t, clone.Env, "BAZ")
	assert.Equal(t, "changed", clone.Env["FOO"])
}

// TestWithoutAuthProviderEnvRestoresPriorValues checks that values set through SetAuthProviderEnv,
// across several calls, are undone to what Env held before the first call, leaving other keys alone.
func TestWithoutAuthProviderEnvRestoresPriorValues(t *testing.T) {
	t.Parallel()

	v := &venv.Venv{
		Env: map[string]string{"SHARED": "original", "OTHER": "kept"},
	}

	v.SetAuthProviderEnv(map[string]string{"SHARED": "first", "ADDED": "first"})
	v.SetAuthProviderEnv(map[string]string{"SHARED": "second"})
	v.Env["LATER"] = "set-after-auth"

	assert.Equal(t, map[string]string{
		"SHARED": "second",
		"ADDED":  "first",
		"OTHER":  "kept",
		"LATER":  "set-after-auth",
	}, v.Env)

	clean := v.WithoutAuthProviderEnv()

	assert.Equal(t, map[string]string{
		"SHARED": "original",
		"OTHER":  "kept",
		"LATER":  "set-after-auth",
	}, clean.Env)
	assert.Equal(t, "second", v.Env["SHARED"], "the original Env must be left untouched")

	clean.SetAuthProviderEnv(map[string]string{"ADDED": "dependency"})

	assert.Equal(t, "original", clean.WithoutAuthProviderEnv().Env["SHARED"])
	assert.NotContains(t, clean.WithoutAuthProviderEnv().Env, "ADDED")
	assert.NotContains(t, v.WithoutAuthProviderEnv().Env, "ADDED")
}

// TestEnviron checks Environ returns sorted KEY=VALUE entries and is the inverse of ParseEnviron.
func TestEnviron(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env  map[string]string
		name string
		want []string
	}{
		{
			name: "empty map",
			env:  map[string]string{},
			want: []string{},
		},
		{
			name: "entries are sorted",
			env:  map[string]string{"ZED": "last", "ALPHA": "first", "MID": "middle"},
			want: []string{"ALPHA=first", "MID=middle", "ZED=last"},
		},
		{
			name: "value with separator and empty value",
			env:  map[string]string{"URL": "https://example.com/?a=b", "EMPTY": ""},
			want: []string{"EMPTY=", "URL=https://example.com/?a=b"},
		},
		{
			name: "windows per-drive key",
			env:  map[string]string{`=C:`: `C:\Users\alice`, "PATH": `C:\bin`},
			want: []string{`=C:=C:\Users\alice`, `PATH=C:\bin`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := venv.Environ(tc.env)

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.env, venv.ParseEnviron(got))
		})
	}
}
