package config

import (
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// EnvRewrite is a temporary, undocumented environment variable that switches discovery to this package's parser.
// Do NOT rely on it: it can be removed or have its name changed at any time without notice and is not part of
// Terragrunt's user-facing configuration surface.
const EnvRewrite = "TG_TMP_CONFIG_REWRITE"

// Enabled reports whether env asks for this package's parser through [EnvRewrite]. A value that is not a boolean
// leaves it off.
func Enabled(env map[string]string) bool {
	venv.RequireEnvMap(env)

	enabled, err := strconv.ParseBool(env[EnvRewrite])

	return err == nil && enabled
}
