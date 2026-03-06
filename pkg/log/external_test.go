// Tests in this file validate that the pkg/log package can be fully utilized as
// an external dependency. Every scenario here imports only public packages
// (pkg/log, pkg/log/format, pkg/log/format/placeholders, pkg/log/writer) and
// never reaches into internal/ packages. If any of these tests fail to compile,
// it signals a broken public API contract.
package log_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalLoggerLifecycle exercises the core create → configure → log →
// clone workflow that an external consumer would follow.
func TestExternalLoggerLifecycle(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	logger := log.New(
		log.WithLevel(log.DebugLevel),
		log.WithOutput(buf),
	)

	// Basic logging methods.
	logger.Info("info message")
	assert.Contains(t, buf.String(), "info message")

	buf.Reset()

	logger.Debugf("count=%d", 42)
	assert.Contains(t, buf.String(), "count=42")

	buf.Reset()

	// Clone and change level independently.
	child := logger.Clone()
	child.SetOptions(log.WithOutput(buf))

	require.NoError(t, child.SetLevel("trace"))
	child.Trace("trace message")
	assert.Contains(t, buf.String(), "trace message")
}

// TestExternalLevelRoundTrip confirms that an external consumer can parse a
// level string, inspect its various name forms, and marshal/unmarshal it.
func TestExternalLevelRoundTrip(t *testing.T) {
	t.Parallel()

	level, err := log.ParseLevel("warn")
	require.NoError(t, err)
	assert.Equal(t, log.WarnLevel, level)
	assert.Equal(t, "warn", level.String())
	assert.Equal(t, "wrn", level.ShortName())
	assert.Equal(t, "w", level.TinyName())

	data, err := level.MarshalText()
	require.NoError(t, err)

	var restored log.Level

	require.NoError(t, restored.UnmarshalText(data))
	assert.Equal(t, level, restored)
}

// TestExternalAllLevelsEnumeration verifies that AllLevels is accessible and
// that every level can be stringified.
func TestExternalAllLevelsEnumeration(t *testing.T) {
	t.Parallel()

	assert.Len(t, log.AllLevels, 7)

	for _, lvl := range log.AllLevels {
		assert.NotEmpty(t, lvl.String())
		assert.True(t, log.AllLevels.Contains(lvl))
	}

	assert.False(t, log.AllLevels.Contains(log.Level(99)))
}

// TestExternalFieldsAndErrors confirms that WithField, WithFields, and
// WithError return enriched loggers whose output contains the added metadata.
func TestExternalFieldsAndErrors(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	logger := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	logger.WithField("component", "auth").Info("field check")
	assert.Contains(t, buf.String(), "component")

	buf.Reset()

	logger.WithFields(log.Fields{"a": 1, "b": 2}).Info("fields check")

	output := buf.String()
	assert.Contains(t, output, "a")
	assert.Contains(t, output, "b")

	buf.Reset()

	logger.WithError(assert.AnError).Info("error check")
	assert.Contains(t, buf.String(), assert.AnError.Error())
}

// TestExternalContextPropagation stores a logger in a context and retrieves it,
// the way middleware or request handlers would.
func TestExternalContextPropagation(t *testing.T) {
	t.Parallel()

	logger := log.New(log.WithLevel(log.InfoLevel))
	ctx := log.ContextWithLogger(t.Context(), logger)

	retrieved := log.LoggerFromContext(ctx)
	require.NotNil(t, retrieved)
	assert.Equal(t, log.InfoLevel, retrieved.Level())

	assert.Nil(t, log.LoggerFromContext(t.Context()))
}

// TestExternalFormatterIntegration creates a Formatter from the format
// subpackage, wires it into a logger, and verifies formatted output.
func TestExternalFormatterIntegration(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(format.NewBareFormatPlaceholders())
	fmtr.SetDisabledColors(true)

	buf := new(bytes.Buffer)
	logger := log.New(
		log.WithLevel(log.InfoLevel),
		log.WithOutput(buf),
		log.WithFormatter(fmtr),
	)

	logger.Info("formatted output")
	assert.Contains(t, buf.String(), "formatted output")
}

// TestExternalParseFormatPresets confirms all four named format presets are
// accessible via ParseFormat.
func TestExternalParseFormatPresets(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		format.BareFormatName,
		format.PrettyFormatName,
		format.JSONFormatName,
		format.KeyValueFormatName,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			phs, err := format.ParseFormat(name)
			require.NoError(t, err)
			assert.NotEmpty(t, phs)
		})
	}
}

// TestExternalCustomFormatParsing parses a user-supplied format string through
// the placeholders subpackage and formats an entry with it.
func TestExternalCustomFormatParsing(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)
	fmtr.SetDisabledColors(true)

	require.NoError(t, fmtr.SetCustomFormat("%level %msg"))

	buf := new(bytes.Buffer)
	logger := log.New(
		log.WithLevel(log.InfoLevel),
		log.WithOutput(buf),
		log.WithFormatter(fmtr),
	)

	logger.Info("custom format test")

	output := buf.String()
	assert.Contains(t, output, "info")
	assert.Contains(t, output, "custom format test")
}

// TestExternalPlaceholderRegistry ensures the placeholder register and its
// Parse function are accessible to external callers.
func TestExternalPlaceholderRegistry(t *testing.T) {
	t.Parallel()

	reg := placeholders.NewPlaceholderRegister()
	assert.NotNil(t, reg.Get("level"))
	assert.NotNil(t, reg.Get("msg"))
	assert.NotNil(t, reg.Get("time"))
	assert.NotNil(t, reg.Get("interval"))
	assert.Nil(t, reg.Get("nonexistent"))

	phs, err := placeholders.Parse("%level %msg")
	require.NoError(t, err)
	assert.NotEmpty(t, phs)
}

// TestExternalWriterAdapter verifies that the writer subpackage can be used to
// bridge an io.Writer into the logging system.
func TestExternalWriterAdapter(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	logger := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

	w := writer.New(
		writer.WithLogger(logger),
		writer.WithDefaultLevel(log.InfoLevel),
		writer.WithMsgSeparator("\n"),
	)

	n, err := w.Write([]byte("line1\nline2"))
	require.NoError(t, err)
	assert.Len(t, "line1\nline2", n)
	assert.Contains(t, buf.String(), "line1")
	assert.Contains(t, buf.String(), "line2")
}

// TestExternalWriterParseFunc confirms that a custom parse function can be
// supplied to the writer adapter.
func TestExternalWriterParseFunc(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	logger := log.New(log.WithLevel(log.TraceLevel), log.WithOutput(buf))
	warnLevel := log.WarnLevel

	w := writer.New(
		writer.WithLogger(logger),
		writer.WithParseFunc(func(str string) (string, *time.Time, *log.Level, error) {
			return "wrapped: " + str, nil, &warnLevel, nil
		}),
	)

	_, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "wrapped: hello")
}

// TestExternalANSIUtilities exercises the ANSI helper functions that external
// consumers might use when processing coloured log output.
func TestExternalANSIUtilities(t *testing.T) {
	t.Parallel()

	coloured := "\033[31mred text\033[0m"
	assert.Equal(t, "red text", log.RemoveAllASCISeq(coloured))

	partial := "\033[32mgreen"
	assert.Contains(t, log.ResetASCISeq(partial), "\033[0m")

	assert.Equal(t, "plain", log.RemoveAllASCISeq("plain"))
	assert.Equal(t, "plain", log.ResetASCISeq("plain"))
}

// TestExternalPackageLevelFunctions confirms the package-level convenience
// functions work without creating an explicit logger.
func TestExternalPackageLevelFunctions(t *testing.T) {
	t.Parallel()

	logger := log.Default()
	require.NotNil(t, logger)

	// WithOptions returns a new logger without mutating the default.
	custom := log.WithOptions(log.WithLevel(log.TraceLevel))
	assert.Equal(t, log.TraceLevel, custom.Level())

	// WithField / WithFields / WithError return enriched loggers.
	assert.NotNil(t, log.WithField("k", "v"))
	assert.NotNil(t, log.WithFields(log.Fields{"a": 1}))
	assert.NotNil(t, log.WithError(assert.AnError))
}

// TestExternalForceLogLevelHook verifies that the force-level hook can be
// created and wired in via WithHooks.
func TestExternalForceLogLevelHook(t *testing.T) {
	t.Parallel()

	hook := log.NewForceLogLevelHook(log.WarnLevel)
	assert.Len(t, hook.Levels(), 7)

	buf := new(bytes.Buffer)

	// The hook can be attached through the public WithHooks option.
	logger := log.New(
		log.WithLevel(log.TraceLevel),
		log.WithOutput(buf),
		log.WithHooks(hook),
	)

	logger.Info("hooked message")
	// The message should still appear (hook changes level, dropper formatter
	// controls visibility based on logger level which is Trace — most permissive).
	assert.Contains(t, buf.String(), "hooked message")
}

// TestExternalConstants ensures exported constants from the package are
// accessible.
func TestExternalConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ".", log.CurDir)
	assert.NotEmpty(t, log.CurDirWithSeparator)

	assert.Equal(t, "bare", format.BareFormatName)
	assert.Equal(t, "pretty", format.PrettyFormatName)
	assert.Equal(t, "json", format.JSONFormatName)
	assert.Equal(t, "key-value", format.KeyValueFormatName)

	assert.Equal(t, "prefix", placeholders.WorkDirKeyName)
	assert.Equal(t, "tf-path", placeholders.TFPathKeyName)
	assert.Equal(t, "tf-command-args", placeholders.TFCmdArgsKeyName)
	assert.Equal(t, "tf-command", placeholders.TFCmdKeyName)
}
