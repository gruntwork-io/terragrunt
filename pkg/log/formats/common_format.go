package formats

import (
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const ansiSeq = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var (
	// regexp matches ansi characters getting from a shell output, used for colors etc.
	ansiReg = regexp.MustCompile(ansiSeq)
)

type CommonFormat struct {
	name     string
	baseTime time.Time

	predefinedPresets Presets
	preset            *Preset

	opts Options
}

func NewCommonFormat(name string, presets Presets) *CommonFormat {
	return &CommonFormat{
		name:              name,
		predefinedPresets: presets,
		preset:            presets.Find(""),
		baseTime:          time.Now(),
	}
}

func (format *CommonFormat) Name() string {
	return format.name
}

func (format *CommonFormat) UsePreset(presetName string) error {
	if preset := format.predefinedPresets.Find(presetName); preset != nil {
		format.preset = preset
	} else if presetName != "" {
		return errors.Errorf("invalid preset %q for the format %q, supported presets: %s", presetName, format.Name(), strings.Join(format.predefinedPresets.Names(), ", "))
	}

	return nil
}

func (format *CommonFormat) SetOptions(opts ...*Option) error {
	format.opts = opts

	return nil

}

func (format *CommonFormat) getOption(name string, level log.Level) *Option {
	var (
		enable *bool
		layout *Layout
	)

	if format.preset != nil {
		if col := format.preset.opts.Get(name, level); col != nil {
			enable = &col.enable
			layout = col.layout
		}
	}

	if col := format.opts.Get(name, level); col != nil {
		enable = &col.enable
		if col.layout != nil {
			layout = col.layout
		}
	}

	if enable == nil {
		return nil
	}

	return NewOption(name, *enable, layout)
}

func (format *CommonFormat) formatOption(name string, curTime time.Time, level log.Level, msg string, fields log.Fields) (string, bool) {
	col := format.getOption(name, level)
	if col == nil || col.layout == nil {
		return "", true
	}

	if !col.enable {
		return "", false
	}

	return col.layout.Value(format.baseTime, curTime, level, msg, fields), true
}

// func (format *CommonFormat) formatLevel(level log.Level, upperCase bool) string {
// 	col := format.getOption(ColLevelName, level)
// 	if !col.enable {
// 		return ""
// 	}

// 	levelFormat := defaultLevelLayout
// 	if col.layout != "" {
// 		levelFormat = col.layout
// 	}

// 	var (
// 		long   = fmt.Sprintf("%-6s", level.String())
// 		middle = strings.ToUpper(level.MiddleName())
// 		short  = strings.ToUpper(level.ShortName())
// 	)

// 	if upperCase {
// 		long = strings.ToUpper(long)
// 		middle = strings.ToUpper(middle)
// 		short = strings.ToUpper(short)
// 	}

// 	levelFormat = strings.ReplaceAll(levelFormat, OptLevelLong, long)
// 	levelFormat = strings.ReplaceAll(levelFormat, ColLevelMiddle, middle)
// 	levelFormat = strings.ReplaceAll(levelFormat, OptLevelShort, short)

// 	return levelFormat
// }

// func (format *CommonFormat) formatPrefix(level log.Level, prefix any, hideCurDir bool) string {
// 	col := format.getOption(OptPrefixName, level)
// 	if !col.enable {
// 		return ""
// 	}

// 	var prefixFormat = defaultPrefixFormat
// 	if col.layout != "" {
// 		prefixFormat = col.layout
// 	}

// 	var absPath, relPath string

// 	switch prefix := prefix.(type) {
// 	case string:
// 		relPath = prefix
// 		absPath = prefix
// 	case func() (string, string):
// 		absPath, relPath = prefix()
// 	}

// 	if hideCurDir && strings.HasPrefix(relPath, log.CurDirWithSeparator) {
// 		relPath = relPath[len(log.CurDirWithSeparator):]
// 	}

// 	prefixFormat = strings.ReplaceAll(prefixFormat, OptPrefixRelPath, relPath)
// 	prefixFormat = strings.ReplaceAll(prefixFormat, OptPrefixAbsPath, absPath)
// 	prefixFormat = strings.ReplaceAll(prefixFormat, OptPrefixDirName, filepath.Base(absPath))

// 	return prefixFormat
// }

// func (format *CommonFormat) formatMessage(level log.Level, message string) string {
// 	if col := format.getOption(OptColorName, level); !col.enable {
// 		message = ansiReg.ReplaceAllString(message, "")
// 	}

// 	return message
// }
