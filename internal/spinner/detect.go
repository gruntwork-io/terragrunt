package spinner

import (
	"bytes"
	"slices"
	"strings"
)

// minWidth is the narrowest terminal a progress line is drawn on.
const minWidth = 20

// windows is the GOOS value of the one platform whose consoles set no TERM.
const windows = "windows"

// dotRows is how many dots high a letter is: the rows of a six-dot braille cell.
const dotRows = 3

// holdFrames is how many frames the letters stay in place before they fall away.
const holdFrames = 6

// blankCell is the braille cell with no dot raised.
const blankCell = rune(0x2800)

// cellDots are the bits a braille cell raises its dots with, by column of the
// cell, then by row.
var cellDots = [...][dotRows]rune{
	{0x01, 0x02, 0x04},
	{0x08, 0x10, 0x20},
}

// letters are T and G, which fall onto the progress line one after the other,
// drawn as rows of dots where '#' is a raised dot. Each is as wide as a whole
// number of braille cells.
var letters = [][dotRows]string{
	{
		"###.",
		".#..",
		".#..",
	},
	{
		"###.",
		"#.##",
		"####",
	},
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
// place one row of dots at a time, the bottom row first. Once all of them are
// in place they hold, then fall away through the bottom of the line.
func fallingFrames(letters [][dotRows]string) []string {
	var canvas [dotRows][]byte

	for _, letter := range letters {
		for row := range dotRows {
			canvas[row] = append(canvas[row], bytes.Repeat([]byte{'.'}, len(letter[row]))...)
		}
	}

	var frames []string

	left := 0

	for _, letter := range letters {
		for row := dotRows - 1; row >= 0; row-- {
			for at := range row + 1 {
				falling := cloneCanvas(canvas)
				copy(falling[at][left:], letter[row])

				frames = append(frames, brailleCells(falling))
			}

			copy(canvas[row][left:], letter[row])
		}

		left += len(letter[0])
	}

	for range holdFrames {
		frames = append(frames, brailleCells(canvas))
	}

	for drop := 1; drop <= dotRows; drop++ {
		frames = append(frames, brailleCells(shiftDown(canvas, drop)))
	}

	return frames
}

// cloneCanvas returns a copy of `canvas` that can be drawn on without changing it.
func cloneCanvas(canvas [dotRows][]byte) [dotRows][]byte {
	var clone [dotRows][]byte

	for row := range dotRows {
		clone[row] = slices.Clone(canvas[row])
	}

	return clone
}

// shiftDown returns `canvas` moved down by `drop` rows, with what moves past
// the bottom row gone and the rows above it blank.
func shiftDown(canvas [dotRows][]byte, drop int) [dotRows][]byte {
	var shifted [dotRows][]byte

	for row := range dotRows {
		shifted[row] = bytes.Repeat([]byte{'.'}, len(canvas[row]))

		if row >= drop {
			copy(shifted[row], canvas[row-drop])
		}
	}

	return shifted
}

// brailleCells returns `canvas` as braille cells, each one two columns of dots wide.
func brailleCells(canvas [dotRows][]byte) string {
	var cells strings.Builder

	for left := 0; left < len(canvas[0]); left += len(cellDots) {
		cell := blankCell

		for column, bits := range cellDots {
			for row, bit := range bits {
				if left+column < len(canvas[row]) && canvas[row][left+column] == '#' {
					cell |= bit
				}
			}
		}

		cells.WriteRune(cell)
	}

	return cells.String()
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
