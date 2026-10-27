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

// SetAuthProviderEnv writes the environment variables an auth provider command
// returned into Env, remembering the values they replace so
// [Venv.WithoutAuthProviderEnv] can undo them. It panics with
// [ErrVenvEnvUnset], per [Venv.RequireEnv], when Env is nil.
func (v *Venv) SetAuthProviderEnv(envs map[string]string) {
	v.RequireEnv()

	prior := maps.Clone(v.authProviderPriorEnv)
	if prior == nil {
		prior = make(map[string]*string, len(envs))
	}

	for key, val := range envs {
		if _, recorded := prior[key]; !recorded {
			if old, ok := v.Env[key]; ok {
				prior[key] = &old
			} else {
				prior[key] = nil
			}
		}

		v.Env[key] = val
	}

	v.authProviderPriorEnv = prior
}

// WithoutAuthProviderEnv returns a copy of v whose Env is an independent clone
// with every value set through [Venv.SetAuthProviderEnv] restored to what it
// was before, so one unit's auth provider values do not reach another unit.
// It panics with [ErrVenvEnvUnset], per [Venv.RequireEnv], when Env is nil.
func (v *Venv) WithoutAuthProviderEnv() *Venv {
	c := v.WithEnvCloned()

	for key, old := range v.authProviderPriorEnv {
		if old == nil {
			delete(c.Env, key)
		} else {
			c.Env[key] = *old
		}
	}

	c.authProviderPriorEnv = nil

	return c
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
