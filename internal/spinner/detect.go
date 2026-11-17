package spinner

import (
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

var ciMarkers = []string{"TF_BUILD", "TEAMCITY_VERSION", "JENKINS_URL", "TF_IN_AUTOMATION"}

// animatable reports whether the stderr of v can show a progress line next to logs for humans.
func animatable(v *venv.Venv, opts Options) bool {
	if !opts.LogsForHumans || !v.Terminal.StderrIsTTY() {
		return false
	}

	if !v.Terminal.StderrHandlesEscapes() || v.Terminal.ErrWidth() < minWidth {
		return false
	}

	term := v.Env["TERM"]
	if strings.EqualFold(term, "dumb") {
		return false
	}

	if v.Platform.GOOS != "windows" && term == "" {
		return false
	}

	return !inCI(v.Env)
}

// frames returns Unicode frames for UTF-8 terminals and Windows Terminal, ASCII otherwise.
func frames(env map[string]string, goos string) []string {
	if goos == "windows" {
		if env["WT_SESSION"] != "" {
			return slices.Clone(unicodeFrames)
		}

		return slices.Clone(asciiFrames)
	}

	locale := strings.ToLower(firstSet(env, "LC_ALL", "LC_CTYPE", "LANG"))
	if strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8") {
		return slices.Clone(unicodeFrames)
	}

	return slices.Clone(asciiFrames)
}

// inCI reports whether CI is set to anything but false, 0, no or off, or a CI marker variable is set.
func inCI(env map[string]string) bool {
	switch strings.ToLower(strings.TrimSpace(env["CI"])) {
	case "", "0", "false", "no", "off":
	default:
		return true
	}

	return slices.ContainsFunc(ciMarkers, func(name string) bool {
		return env[name] != ""
	})
}

// firstSet returns the first non-empty value of names in env, or "" when none is set.
func firstSet(env map[string]string, names ...string) string {
	for _, name := range names {
		if value := env[name]; value != "" {
			return value
		}
	}

	return ""
}
