package format

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/config"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	optionSeparator = ","

	columnPrefix    = "#"
	columnPrefixLen = len(columnPrefix)

	defaultConfigName     = ""
	defaultQuoteCharacter = "\""
)

var (
	optionSeparatorReg = regexp.MustCompile(`.*?[^\\](` + optionSeparator + `|$)`)
)

const (
	OptionColor    = "color"
	OptionKeyValue = "key-value"
	OptionJSON     = "json"
	OptionIndent   = "indent"
)

type Formatter struct {
	baseTime time.Time

	presetConfigs      config.Configs
	selectedConfigName string

	userOpts config.Options

	quoteEmptyFields bool

	quoteCharacter string
}

func NewFormatter(opts ...Option) *Formatter {
	formatter := &Formatter{
		presetConfigs: config.Configs{DefaultConfig},

		baseTime:           time.Now(),
		selectedConfigName: defaultConfigName,

		quoteCharacter: defaultQuoteCharacter,
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

	if configName := formatter.selectedConfigName; configName != "" {
		strs = append(strs, configName)
	}

	// get all non-column options
	opts := formatter.Options().FilterByNamePrefixes(false, columnPrefix)

	strs = append(strs, opts.Names()...)
	strs = util.RemoveDuplicatesFromList(strs)

	return strings.Join(strs, ",")
}

func (formatter *Formatter) Options() config.Options {
	var opts config.Options

	if preset := formatter.presetConfigs.Find(formatter.selectedConfigName); preset != nil {
		opts = preset.Options()
	}

	return append(opts, formatter.userOpts...)
}

// SetFormat parses options in the given `str` and sets them to the formatter.
func (formatter *Formatter) SetFormat(str string) error {
	parts := optionSeparatorReg.FindAllString(str, -1)
	for i, str := range parts {
		if i < len(parts)-1 {
			str = str[:len(str)-1]
		}
		str = strings.ReplaceAll(str, `\`+optionSeparator, optionSeparator)

		if selectedConfig := formatter.presetConfigs.Find(str); selectedConfig != nil {
			formatter.selectedConfigName = selectedConfig.Name()
			continue
		}

		opt, err := config.ParseOption(str)
		if err != nil {
			return err
		}

		formatter.userOpts = append(formatter.userOpts, opt)
	}

	return nil
}

func (formatter *Formatter) GetOption(name string, levels ...log.Level) *config.Option {
	var opts config.Options

	if preset := formatter.presetConfigs.Find(formatter.selectedConfigName); preset != nil {
		if opt := preset.Options().FindByName(name).MergeIntoOneWithPriorityByLevels(levels...); opt != nil {
			opts = append(opts, opt)
		}
	}

	if opt := formatter.userOpts.FindByName(name).MergeIntoOneWithPriorityByLevels(levels...); opt != nil {
		opts = append(opts, opt)
	}

	return opts.MergeIntoOne()
}

func (formatter *Formatter) getOptionsByNamePrefixAndLevel(prefix string, levels ...log.Level) config.Options {
	optsNames := formatter.Options().FilterByNamePrefixes(true, columnPrefix).Names()
	optsNames = util.RemoveDuplicatesFromList(optsNames)

	opts := make(config.Options, len(optsNames))

	for i, optName := range optsNames {
		opts[i] = formatter.GetOption(optName, levels...)
	}

	return opts
}

func (formatter *Formatter) optionValue(name string, level log.Level, entry *config.Entry) (string, bool) {
	opt := formatter.GetOption(name, level)
	if opt == nil {
		return "", true
	}

	return opt.Value(entry), opt.Enable()
}

// Format implements logrus.Formatter
func (formatter *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	var (
		colsFields     = make(log.Fields)
		colsNames      []string
		colsValues     []string
		level          = log.FromLogrusLevel(entry.Level)
		fields         = log.Fields(entry.Data)
		msg            = entry.Message
		curTime        = entry.Time
		disableColor   bool
		jsonFormat     bool
		keyValueFormat bool
	)

	if opt := formatter.GetOption(OptionColor, level); opt != nil && !opt.Enable() {
		disableColor = true
	}

	if opt := formatter.GetOption(OptionJSON, level); opt != nil && opt.Enable() {
		disableColor = true
		jsonFormat = true
	}

	if opt := formatter.GetOption(OptionKeyValue, level); opt != nil && opt.Enable() {
		disableColor = true
		keyValueFormat = true
	}

	presetEntry := config.NewEntry(formatter.baseTime, curTime, level, msg, fields, disableColor)

	opts := formatter.getOptionsByNamePrefixAndLevel(columnPrefix, level).MergeByName().SortByValue()

	for _, opt := range opts {
		if !opt.Enable() {
			continue
		}
		if val := opt.Value(presetEntry); val != "" {
			if disableColor {
				val = log.RemoveAllASCISeq(val)
			}

			colName := opt.Name()[columnPrefixLen:]
			colsNames = append(colsNames, colName)
			colsValues = append(colsValues, val)
			colsFields[colName] = val
		}
	}

	for key, value := range fields {
		if val, ok := formatter.optionValue(key, level, presetEntry); !ok {
			delete(fields, key)
			continue
		} else if val != "" {
			fields[key] = val
			continue
		}

		if val, ok := value.(string); ok && disableColor {
			fields[key] = log.RemoveAllASCISeq(val)
		}
	}

	if len(colsValues) == 0 && len(fields) == 0 {
		return nil, nil
	}

	if keyValueFormat {
		return formatter.keyValueFormat(buf, level, colsNames, colsFields, fields)
	}

	if jsonFormat {
		return formatter.jsonFormat(buf, level, fields, colsFields)
	}

	return formatter.textFormat(buf, fields, colsValues)
}

func (formatter *Formatter) textFormat(buf *bytes.Buffer, fields log.Fields, colsValues []string) ([]byte, error) {
	if _, err := fmt.Fprint(buf, strings.Join(colsValues, " ")); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	for _, key := range fields.Keys() {
		value := fields[key]
		if err := formatter.appendKeyValue(buf, key, value); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) keyValueFormat(buf *bytes.Buffer, level log.Level, colsNames []string, colsFields, fields log.Fields) ([]byte, error) {
	for _, key := range colsNames {
		val, ok := colsFields[key]
		if !ok {
			continue
		}
		if err := formatter.appendKeyValue(buf, key, val); err != nil {
			return nil, err
		}
	}

	for _, key := range fields.Keys() {
		val := fields[key]
		if err := formatter.appendKeyValue(buf, key, val); err != nil {
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

func (formatter *Formatter) appendKeyValue(buf *bytes.Buffer, key string, value interface{}) error {
	if err := formatter.appendKey(buf, key); err != nil {
		return err
	}

	if err := formatter.appendValue(buf, value); err != nil {
		return err
	}

	return nil
}

func (format *Formatter) appendKey(buf *bytes.Buffer, key interface{}) error {
	keyFmt := "%s="
	if buf.Len() > 0 {
		keyFmt = " " + keyFmt
	}

	if _, err := fmt.Fprintf(buf, keyFmt, key); err != nil {
		return errors.WithStackTrace(err)
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
		valueFmt = format.quoteCharacter + valueFmt + format.quoteCharacter
	}

	if _, err := fmt.Fprintf(buf, valueFmt, value); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (format *Formatter) needsQuoting(text string) bool {
	if format.quoteEmptyFields && len(text) == 0 {
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
