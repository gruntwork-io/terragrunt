package exec

import "slices"

// EnvSliceFromMap converts an env-var map to the KEY=VALUE slice form
// expected by os/exec.Cmd.Env. The result is sorted for deterministic output.
func EnvSliceFromMap(env map[string]string) []string {
	out := make([]string, 0, len(env))

	for k, v := range env {
		out = append(out, k+"="+v)
	}

	slices.Sort(out)

	return out
}
