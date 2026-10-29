package spinner

import (
	"slices"
	"strings"
)

const minWidth = 20

var (
	unicodeFrames = []string{
		"⠁", "⠂", "⠄", "⠍", "⠖", "⠞", "⠞", "⠞", "⠞", "⠞", "⠞", "⠴", "⠠", "⠀",
		"⠉", "⠒", "⠛", "⠛", "⠛", "⠛", "⠛", "⠛", "⠶", "⠤", "⠀",
	}
	asciiFrames = []string{"|", "/", "-", "\\"}
	ciMarkers   = []string{"TF_BUILD", "TEAMCITY_VERSION", "JENKINS_URL", "TF_IN_AUTOMATION"}
)

// animatable reports whether the progress line can be drawn.
func animatable(opts *Options) bool {
	if !opts.OutIsTTY || !opts.LogsForHumans {
		return false
	}

	if opts.Out == nil || opts.Width == nil || opts.Width() < minWidth {
		return false
	}

	term := opts.Env["TERM"]
	if strings.EqualFold(term, "dumb") {
		return false
	}

	if opts.GOOS != "windows" && term == "" {
		return false
	}

	return !inCI(opts.Env)
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

// inCI reports whether env is from a CI system.
func inCI(env map[string]string) bool {
	ci := strings.TrimSpace(env["CI"])
	if ci != "" && ci != "0" && !strings.EqualFold(ci, "false") {
		return true
	}

	return slices.ContainsFunc(ciMarkers, func(name string) bool {
		return env[name] != ""
	})
}

func firstSet(env map[string]string, names ...string) string {
	for _, name := range names {
		if value := env[name]; value != "" {
			return value
		}
	}

	return ""
}
