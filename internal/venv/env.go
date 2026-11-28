package venv

import (
	"maps"
	"slices"
	"strings"
)

// WithEnv returns a copy of v whose shell environment is env. It panics
// with [ErrVenvEnvNil] on a nil env so the result always satisfies
// [Venv.RequireEnv].
func (v *Venv) WithEnv(env map[string]string) *Venv {
	if env == nil {
		panic(ErrVenvEnvNil)
	}

	c := *v
	c.Env = env

	return &c
}

// WithEnvCloned returns a copy of v whose Env is an independent clone. Fan-out
// paths that process units one at a time hand each unit a clone so
// per-unit mutations (obtained credentials, TF_VAR_* contributions) never
// leak into sibling units. It panics with [ErrVenvEnvUnset], per
// [Venv.RequireEnv], when Env is nil.
func (v *Venv) WithEnvCloned() *Venv {
	v.RequireEnv()

	return v.WithEnv(maps.Clone(v.Env))
}

// RequireEnv panics with [ErrVenvEnvUnset] when Env is nil, guarding
// functions that write into the shared environment.
func (v *Venv) RequireEnv() {
	if v.Env == nil {
		panic(ErrVenvEnvUnset)
	}
}

// RequireEnvMap panics with [ErrVenvEnvUnset] when env is nil. A nil map reads
// as empty rather than failing, so a function handed one would resolve every
// lookup to "" and report a run that simply found no environment. Callers that
// take an environment as a parameter assert it here.
func RequireEnvMap(env map[string]string) {
	if env == nil {
		panic(ErrVenvEnvUnset)
	}
}

// Environ turns an environment map back into os.Environ-style KEY=VALUE
// entries, sorted so the result does not vary with map iteration order. It is
// the inverse of [ParseEnviron].
func Environ(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}

	slices.Sort(out)

	return out
}

// ParseEnviron turns os.Environ-style KEY=VALUE entries into a map, splitting
// on the first "=" after the leading byte. That leading byte is skipped so the
// Windows per-drive working-directory variables, whose names begin with "="
// (e.g. "=C:"), keep their names intact. Entries without a separator are dropped.
func ParseEnviron(environ []string) map[string]string {
	out := make(map[string]string, len(environ))

	for _, entry := range environ {
		if entry == "" {
			continue
		}

		i := strings.IndexByte(entry[1:], '=')
		if i < 0 {
			continue
		}

		out[entry[:i+1]] = entry[i+2:]
	}

	return out
}
