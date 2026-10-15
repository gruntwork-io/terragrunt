package spinner

import (
	"slices"
	"strings"
)

// minWidth is the narrowest terminal a progress line is drawn on.
const minWidth = 20

// windows is the GOOS value of the one platform whose consoles set no TERM.
const windows = "windows"

// letterRows and letterColumns are the size of a letter of the animation, in dots.
const (
	letterRows    = 4
	letterColumns = 4
)

// cellColumns is how many columns of dots one braille character holds.
const cellColumns = 2

// brailleBlank is the braille character with no dot raised, which the dots of a cell are added to.
const brailleBlank = '\u2800'

// holdFrames is how many frames a finished letter is shown before its dots start to change.
const holdFrames = 5

// letterT and letterG are the letters the animation changes between, one string per row of dots.
var (
	letterT = [letterRows]string{"####", ".##.", ".##.", ".##."}
	letterG = [letterRows]string{".###", "#...", "#.##", ".###"}
)

// brailleDots are the bits of a braille character by row, for its left and its right column of dots.
var brailleDots = [letterRows][cellColumns]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

// unicodeFrames animate the progress line on a terminal that shows UTF-8: a T
// drawn in dots changes into a G and back, one dot per frame.
var unicodeFrames = morphFrames(letterT, letterG)

// asciiFrames animate the progress line on every other terminal: the letters T
// and G take turns, in step with the dots.
var asciiFrames = slices.Concat(
	slices.Repeat([]string{"T"}, len(unicodeFrames)/2), //nolint:mnd // each letter gets half of the cycle
	slices.Repeat([]string{"G"}, len(unicodeFrames)/2), //nolint:mnd // each letter gets half of the cycle
)

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

// morphFrames returns the frames that show `from`, change it into `to` one dot
// at a time, show `to`, and change it back.
func morphFrames(from, to [letterRows]string) []string {
	start, end := dotsOf(from), dotsOf(to)

	return slices.Concat(
		slices.Repeat([]string{braille(start)}, holdFrames),
		between(start, end),
		slices.Repeat([]string{braille(end)}, holdFrames),
		between(end, start),
	)
}

// between returns the frames that lie between `from` and `to`: each one has
// one more dot of `to` than the frame before it, from the top left dot down.
func between(from, to uint16) []string {
	var frames []string

	dots := from

	for dot := range letterRows * letterColumns {
		changed := dots ^ (dots^to)&(1<<dot)
		if changed == dots || changed == to {
			continue
		}

		dots = changed
		frames = append(frames, braille(dots))
	}

	return frames
}

// dotsOf returns the raised dots of `letter` as bits, the top left dot lowest.
func dotsOf(letter [letterRows]string) uint16 {
	var dots uint16

	for row, text := range letter {
		for column, char := range text {
			if char == '#' {
				dots |= 1 << (row*letterColumns + column)
			}
		}
	}

	return dots
}

// braille returns `dots` drawn with braille characters, two columns of dots each.
func braille(dots uint16) string {
	cells := make([]rune, 0, letterColumns/cellColumns)

	for cell := range letterColumns / cellColumns {
		char := brailleBlank

		for row := range letterRows {
			for column := range cellColumns {
				if dots&(1<<(row*letterColumns+cell*cellColumns+column)) != 0 {
					char |= brailleDots[row][column]
				}
			}
		}

		cells = append(cells, char)
	}

	return string(cells)
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
