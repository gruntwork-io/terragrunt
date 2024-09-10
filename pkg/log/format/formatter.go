package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/preset"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	defaultOptionSeparator = ","
	defaultPresetName      = ""
	columnPrefix           = "#"
)

const (
	OptionColor  = "color"
	OptionJSON   = "json"
	OptionIndent = "indent"
)

type Formatter struct {
	optionSeparator string
	baseTime        time.Time

	presets            preset.Presets
	selectedPresetName string

	userOpts preset.Options

	// Wrap empty fields in quotes if true.
	QuoteEmptyFields bool

	// Can be set to the override the default quoting character " with something else. For example: ', or `.
	QuoteCharacter string
}

func NewFormatter(opts ...Option) *Formatter {
	formatter := &Formatter{
		presets: preset.Presets{DefaultPreset},

		baseTime:           time.Now(),
		selectedPresetName: defaultPresetName,
		optionSeparator:    defaultOptionSeparator,
	}

	formatter.SetOption(opts...)

	return formatter
}

func (formatter *Formatter) SetOption(opts ...Option) {
	for _, opt := range opts {
		opt(formatter)
	}
}

// String implements fmt.Stringer
func (formatter *Formatter) String() string {
	var strs []string

	if presetName := formatter.selectedPresetName; presetName != "" {
		strs = append(strs, presetName)
	}

	// get all non-column options
	opts := formatter.Options().FilterByNamePrefixes(false, columnPrefix)

	strs = append(strs, opts.Names()...)

	return strings.Join(strs, ",")
}

func (formatter *Formatter) Options() preset.Options {
	var opts preset.Options

	if preset := formatter.presets.Find(formatter.selectedPresetName); preset != nil {
		opts = preset.Options()
	}

	return append(opts, formatter.userOpts...)
}

// TODO: ParseFormat takes a string and returns a Format instance with defined options.

// pretty:tiny@no-color@ident:%s %s %s@time@level@prefix
func (formatter *Formatter) SetFormat(str string) error {
	parts := strings.Split(str, formatter.optionSeparator)
	for _, str := range parts {
		presetName := parts[0]

		if selectedPreset := formatter.presets.Find(presetName); selectedPreset != nil {
			formatter.selectedPresetName = presetName
			continue
		}

		opt, err := preset.ParseOption(str)
		if err != nil {
			return err
		}

		formatter.userOpts = append(formatter.userOpts, opt)
	}

	return nil
}

func (formatter *Formatter) GetOption(name string, levels ...log.Level) *preset.Option {
	var opts preset.Options

	if preset := formatter.presets.Find(formatter.selectedPresetName); preset != nil {
		if opt := preset.Options().FindByName(name).MergeIntoOneWithPriorityByLevels(levels...); opt != nil {
			opts = append(opts, opt)
		}
	}

	if opt := formatter.userOpts.FindByName(name).MergeIntoOneWithPriorityByLevels(levels...); opt != nil {
		opts = append(opts, opt)
	}

	return opts.MergeIntoOne()
}

func (formatter *Formatter) getOptionsByNamePrefixAndLevel(prefix string, levels ...log.Level) preset.Options {
	optsNames := formatter.Options().FilterByNamePrefixes(true, columnPrefix).Names()
	optsNames = util.RemoveDuplicatesFromList(optsNames)

	opts := make(preset.Options, len(optsNames))

	for i, optName := range optsNames {
		opts[i] = formatter.GetOption(optName, levels...)
	}

	return opts
}

func (formatter *Formatter) optionValue(name string, level log.Level, entry *preset.Entry) (string, bool) {
	opt := formatter.GetOption(name, level)
	if opt == nil {
		return "", true
	}

	return opt.Value(entry)
}

// Format implements logrus.Formatter
func (formatter *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	var (
		cols         []string
		colsFields   = make(log.Fields)
		level        = log.FromLogrusLevel(entry.Level)
		fields       = log.Fields(entry.Data)
		msg          = entry.Message
		curTime      = entry.Time
		disableColor bool
		jsonFormat   bool
	)

	if opt := formatter.GetOption(OptionColor, level); opt != nil && !opt.Enable() {
		disableColor = true
	}

	if opt := formatter.GetOption(OptionJSON, level); opt != nil && opt.Enable() {
		disableColor = true
		jsonFormat = true
	}

	presetEntry := preset.NewEntry(formatter.baseTime, curTime, level, msg, fields, disableColor)

	opts := formatter.getOptionsByNamePrefixAndLevel(columnPrefix, level)
	for _, opt := range opts {
		if val, ok := opt.Value(presetEntry); ok && val != "" {
			if disableColor {
				val = log.RemoveAllASCISeq(val)
			}

			cols = append(cols, val)

			key := opt.Name()[len(columnPrefix):]
			colsFields[key] = val
		}
	}

	for key, value := range fields {
		if val, ok := formatter.optionValue(key, level, presetEntry); !ok {
			delete(fields, key)
		} else if val != "" {
			fields[key] = val
			continue
		}

		if val, ok := value.(string); ok && disableColor {
			fields[key] = log.RemoveAllASCISeq(val)
		}
	}

	if jsonFormat {
		return formatter.jsonFormat(buf, level, fields, colsFields)
	}

	return formatter.textFormat(buf, fields, cols)
}

func (formatter *Formatter) textFormat(buf *bytes.Buffer, fields log.Fields, cols []string) ([]byte, error) {
	if _, err := fmt.Fprint(buf, strings.Join(cols, " ")); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	for _, key := range fields.Keys() {
		value := fields[key]
		if err := formatter.appendKeyValue(buf, key, value, true); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) jsonFormat(buf *bytes.Buffer, level log.Level, fields, colsFields log.Fields) ([]byte, error) {
	encoder := json.NewEncoder(buf)

	if opt := formatter.GetOption(OptionIndent, level); opt != nil && opt.Enable() {
		encoder.SetIndent("", "  ")
	}

	maps.Copy(fields, colsFields)

	if err := encoder.Encode(fields); err != nil {
		return nil, errors.Errorf("failed to marshal fields to JSON, %w", err)
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) appendKeyValue(buf *bytes.Buffer, key string, value interface{}, appendSpace bool) error {
	keyFmt := "%s="
	if appendSpace {
		keyFmt = " " + keyFmt
	}

	if _, err := fmt.Fprintf(buf, keyFmt, key); err != nil {
		return errors.WithStackTrace(err)
	}

	if err := formatter.appendValue(buf, value); err != nil {
		return err
	}

	return nil
}

func (format *Formatter) appendValue(buf *bytes.Buffer, value interface{}) error {
	var str string

	switch value := value.(type) {
	case string:
		str = value
	case error:
		str = value.Error()
	default:
		if _, err := fmt.Fprint(buf, value); err != nil {
			return errors.WithStackTrace(err)
		}

		return nil
	}

	valueFmt := "%v"
	if format.needsQuoting(str) {
		valueFmt = format.QuoteCharacter + valueFmt + format.QuoteCharacter
	}

	if _, err := fmt.Fprintf(buf, valueFmt, value); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (format *Formatter) needsQuoting(text string) bool {
	if format.QuoteEmptyFields && len(text) == 0 {
		return true
	}

	for _, ch := range text {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.') {
			return true
		}
	}

	return false
}
