package formats

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const ansiSeq = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var (
	defaultTimestampFormat = time.RFC3339
	defaultLevelFormat     = OptionLevelLong
	defaultPrefixFormat    = "[" + OptionPrefixRelPath + "]"

	commonSupportedOptions = []string{OptionTime, OptionLevel, OptionPrefix, OptionColor}

	// regexp matches ansi characters getting from a shell output, used for colors etc.
	ansiReg = regexp.MustCompile(ansiSeq)

	timestampFormatMap = map[string]string{
		"Y": "2006",
		"y": "06",
		"m": "01",
		"n": "1",
		"M": "Jan",
		"j": "2",
		"d": "02",
		"D": "Mon",
		"l": "Monday",
		"A": "PM",
		"a": "pm",
		"H": "15",
		"h": "03",
		"g": "3",
		"i": "04",
		"s": "05",
		"u": ".000000",
		"v": ".000",
		"T": "MST",
		"O": "-0700",
		"P": "-07:00",
	}

	timestampFormatStampMap = map[string]string{
		"rfc3339":      time.RFC3339,
		"rfc3339-nano": time.RFC3339Nano,
		"date-time":    time.DateTime,
		"date-only":    time.DateOnly,
		"time-only":    time.TimeOnly,
	}
)

type CommonFormatter struct {
	name          string
	baseTimestamp time.Time

	supportedPresets Presets
	preset           *Preset

	supportedOptions []string
	options          Options
}

func (formatter *CommonFormatter) Name() string {
	return formatter.name
}

func (formatter *CommonFormatter) SetOptions(presetName string, opts ...*Option) error {
	preset := formatter.supportedPresets.Find(presetName)
	if preset == nil && presetName != "" {
		return errors.Errorf("invalid preset %q for the format %q, supported presets: %s", presetName, formatter.Name(), strings.Join(formatter.supportedPresets.Names(), ", "))
	}

	for _, opt := range opts {
		if !collections.ListContainsElement(formatter.supportedOptions, opt.name) {
			return errors.Errorf("invalid option %q for the format %q, supported options: %s", opt.name, formatter.Name(), strings.Join(formatter.supportedOptions, ", "))
		}
	}

	formatter.preset = preset
	formatter.options = opts
	return nil
}

func (formatter *CommonFormatter) getOption(name string, level log.Level) *Option {
	var (
		enable bool
		value  string
	)

	if formatter.preset != nil {
		if opt := formatter.preset.opts.Find(name, level); opt != nil {
			enable = opt.enable
			value = opt.value
		}
	}

	if opt := formatter.options.Find(name, level); opt != nil {
		enable = opt.enable
		if opt.value != "" {
			value = opt.value
		}
	}

	return NewOption(name, enable, value)
}

func (formatter *CommonFormatter) getTimestamp(level log.Level, t time.Time) string {
	opt := formatter.getOption(OptionTime, level)
	if !opt.enable {
		return ""
	}

	timestampFormat := defaultTimestampFormat
	if opt.value != "" {
		timestampFormat = opt.value
	}

	for old, new := range timestampFormatStampMap {
		timestampFormat = strings.ReplaceAll(timestampFormat, "@"+old, new)
	}

	for old, new := range timestampFormatMap {
		timestampFormat = strings.ReplaceAll(timestampFormat, "%"+old, new)
	}

	val := t.Format(timestampFormat)
	val = strings.ReplaceAll(val, "@mini", fmt.Sprintf("%04d", time.Since(formatter.baseTimestamp)/time.Second))

	return val
}

func (formatter *CommonFormatter) getLevel(level log.Level) string {
	opt := formatter.getOption(OptionLevel, level)
	if !opt.enable {
		return ""
	}

	levelFormat := defaultLevelFormat
	if opt.value != "" {
		levelFormat = opt.value
	}

	var (
		long  = fmt.Sprintf("%-6s", level.String())
		short = strings.ToUpper(level.ShortName())
	)

	levelFormat = strings.ReplaceAll(levelFormat, OptionLevelLong, long)
	levelFormat = strings.ReplaceAll(levelFormat, OptionLevelShort, short)

	return strings.ToUpper(levelFormat)
}

func (formatter *CommonFormatter) getPrefix(level log.Level, prefix any) string {
	opt := formatter.getOption(OptionPrefix, level)
	if !opt.enable {
		return ""
	}

	var prefixFormat = defaultPrefixFormat
	if opt.value != "" {
		prefixFormat = opt.value
	}

	var absPath, relPath string

	switch prefix := prefix.(type) {
	case string:
		relPath = prefix
		absPath = prefix
	case func() (string, string):
		absPath, relPath = prefix()
	}

	if strings.HasPrefix(relPath, log.CurDirWithSeparator) {
		relPath = relPath[len(log.CurDirWithSeparator):]
	}

	prefixFormat = strings.ReplaceAll(prefixFormat, OptionPrefixRelPath, relPath)
	prefixFormat = strings.ReplaceAll(prefixFormat, OptionPrefixAbsPath, absPath)

	return prefixFormat
}
