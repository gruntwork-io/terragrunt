package spinner

import (
	"slices"
	"strings"
)

// minWidth is the narrowest terminal a progress line is drawn on.
const minWidth = 20

// windows is the GOOS value of the one platform whose consoles set no TERM.
const windows = "windows"

// unicodeFrames animate the progress line on a terminal that shows UTF-8.
var unicodeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// asciiFrames animate the progress line on every other terminal.
var asciiFrames = []string{"|", "/", "-", "\\"}

// ciMarkers are the variables build systems set, on top of CI itself.
var ciMarkers = []string{"TF_BUILD", "TEAMCITY_VERSION", "JENKINS_URL", "TF_IN_AUTOMATION"}

// animatable reports whether progress can be drawn as a line that is redrawn in place.
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

	if opts.GOOS != windows && term == "" {
		return false
	}

	return !inCI(opts.Env)
}

// frames returns the animation frames the terminal of the run can show.
func frames(env map[string]string, goos string) []string {
	if goos == windows {
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

// inCI reports whether env carries the mark of a build system.
func inCI(env map[string]string) bool {
	ci := strings.TrimSpace(env["CI"])
	if ci != "" && ci != "0" && !strings.EqualFold(ci, "false") {
		return true
	}

	return slices.ContainsFunc(ciMarkers, func(name string) bool {
		return env[name] != ""
	})
}

// firstSet returns the value of the first of names that env sets, or the empty string.
func firstSet(env map[string]string, names ...string) string {
	for _, name := range names {
		if value := env[name]; value != "" {
			return value
		}
	}

	return ""
}
