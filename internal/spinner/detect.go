package spinner

import (
	"slices"
	"strings"
)

// minWidth is the narrowest terminal a progress line is drawn on.
const minWidth = 20

// windows is the GOOS value of the one platform whose consoles set no TERM.
const windows = "windows"

// dotRows is how many rows of dots a six-dot braille cell has.
const dotRows = 3

// holdFrames is how many frames a letter stays in place before it falls away.
const holdFrames = 5

// blankCell is the braille cell with no dot raised.
const blankCell = rune(0x2800)

// cellDots are the bits a braille cell raises its dots with, by row of the
// cell, then by column.
var cellDots = [dotRows][2]rune{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
}

// letters are the braille letters T (U+281E) and G (U+281B), drawn as rows of
// dots where '#' is a raised dot. They fall into the cell one after the other.
var letters = [][dotRows]string{
	{".#", "##", "#."},
	{"##", "##", ".."},
}

// unicodeFrames animate the progress line on a terminal that shows UTF-8.
var unicodeFrames = fallingFrames(letters)

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

// fallingFrames returns frames in which each of `letters` in turn falls into
// the cell one row of dots at a time, the bottom row first, stays in place,
// then falls away through the bottom of the cell.
func fallingFrames(letters [][dotRows]string) []string {
	var frames []string

	for _, letter := range letters {
		var landed rune

		for row := dotRows - 1; row >= 0; row-- {
			if !strings.Contains(letter[row], "#") {
				continue
			}

			for at := range row + 1 {
				frames = append(frames, string(blankCell|landed|rowDots(letter[row], at)))
			}

			landed |= rowDots(letter[row], row)
		}

		for range holdFrames {
			frames = append(frames, string(blankCell|landed))
		}

		for drop := 1; drop <= dotRows; drop++ {
			shifted := blankCell

			for row := range dotRows - drop {
				shifted |= rowDots(letter[row], row+drop)
			}

			frames = append(frames, string(shifted))
		}
	}

	return frames
}

// rowDots returns the bits that raise the dots of `pattern` on row `row` of a cell.
func rowDots(pattern string, row int) rune {
	var bits rune

	for column, bit := range cellDots[row] {
		if pattern[column] == '#' {
			bits |= bit
		}
	}

	return bits
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
