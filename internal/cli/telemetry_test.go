package cli_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/global"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const zeroSpanID = "0000000000000000"

// TestWrapWithTelemetryExportsLogsOfInterruptedRun runs a command whose action
// logs through the root logger and then cancels the run, as an interrupt does,
// and checks the record is still exported and linked to the command span.
func TestWrapWithTelemetryExportsLogsOfInterruptedRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(config.WithCaches(t.Context()))
	defer cancel()

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	testV := venvtest.New()
	testV.Env["TG_TELEMETRY_TRACE_EXPORTER"] = "console"
	testV.Env["TG_TELEMETRY_LOGS_EXPORTER"] = "console"

	terragruntCommands := commands.New(l, opts, testV)
	setCommandAction(func(context.Context, *clihelper.Context) error {
		l.Infof("record of interrupted run")
		cancel()

		return nil
	}, terragruntCommands...)

	stdout := &lockedWriter{}

	app := clihelper.NewApp(testV.Env)
	app.Writer = stdout
	app.ErrWriter = &bytes.Buffer{}

	app.Flags = append(global.NewFlags(l, opts, nil), run.NewFlags(l, opts, testV, nil)...)
	app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts, testV))

	require.NoError(t, app.RunContext(ctx, []string{"terragrunt", "run", "--", "plan"}))

	var (
		records []consoleLogRecord
		rootIDs []string
	)

	scanner := bufio.NewScanner(strings.NewReader(stdout.String()))
	scanner.Buffer(nil, 1<<20)

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, `{"Timestamp":`):
			var record consoleLogRecord
			require.NoError(t, json.Unmarshal([]byte(line), &record), line)

			records = append(records, record)
		case strings.HasPrefix(line, `{"Name":`):
			var span consoleSpan
			require.NoError(t, json.Unmarshal([]byte(line), &span), line)

			if span.Parent.SpanID == zeroSpanID {
				rootIDs = append(rootIDs, span.SpanContext.SpanID)
			}
		}
	}

	require.NoError(t, scanner.Err())
	require.Len(t, rootIDs, 1, "expected exactly one root command span")

	var found []consoleLogRecord

	for _, record := range records {
		if record.Body.Value == "record of interrupted run" {
			found = append(found, record)
		}
	}

	require.Len(t, found, 1, "the record of the interrupted run must be exported once")
	assert.Equal(t, rootIDs[0], found[0].SpanID)
}

// TestWrapWithTelemetryBoundsShutdownWhenExportStalls runs a command whose
// logs export never completes and checks that flushing delays exit by the
// shutdown timeout and no longer.
func TestWrapWithTelemetryBoundsShutdownWhenExportStalls(t *testing.T) {
	t.Parallel()

	// The CLI's telemetry shutdown timeout.
	const shutdownTimeout = 30 * time.Second

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(config.WithCaches(t.Context()))
		defer cancel()

		l := logger.CreateLogger()
		l.SetOptions(log.WithOutput(io.Discard))

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.NoAutoProviderCacheDir = true

		testV := venvtest.New()
		testV.Env["TG_TELEMETRY_LOGS_EXPORTER"] = "console"

		terragruntCommands := commands.New(l, opts, testV)
		setCommandAction(func(context.Context, *clihelper.Context) error {
			l.Infof("record of interrupted run")
			cancel()

			return nil
		}, terragruntCommands...)

		stdout := &stalledWriter{release: make(chan struct{})}
		stderr := &lockedWriter{}

		app := clihelper.NewApp(testV.Env)
		app.Writer = stdout
		app.ErrWriter = stderr

		app.Flags = append(global.NewFlags(l, opts, nil), run.NewFlags(l, opts, testV, nil)...)
		app.Commands = terragruntCommands.WrapAction(commands.WrapWithTelemetry(l, opts, testV))

		start := time.Now()
		err := app.RunContext(ctx, []string{"terragrunt", "run", "--", "plan"})
		elapsed := time.Since(start)

		close(stdout.release)

		require.NoError(t, err)
		assert.Equal(t, shutdownTimeout, elapsed)
		assert.Contains(t, stderr.String(), context.DeadlineExceeded.Error())
	})
}

// consoleLogRecord is the part of a record printed by the console logs
// exporter that these tests read.
type consoleLogRecord struct {
	Body struct {
		Value string `json:"Value"`
	} `json:"Body"`
	TraceID string `json:"TraceID"`
	SpanID  string `json:"SpanID"`
}

// consoleSpan is the part of a span printed by the console trace exporter
// that these tests read.
type consoleSpan struct {
	SpanContext struct {
		SpanID string `json:"SpanID"`
	} `json:"SpanContext"`
	Parent struct {
		SpanID string `json:"SpanID"`
	} `json:"Parent"`
}

// lockedWriter collects output from concurrent writers.
type lockedWriter struct {
	buf strings.Builder
	mu  sync.Mutex
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.Write(p)
}

func (w *lockedWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.String()
}

// stalledWriter blocks every write until release is closed, like a collector
// that accepts a connection but never answers.
type stalledWriter struct {
	release chan struct{}
}

func (w *stalledWriter) Write(p []byte) (int, error) {
	<-w.release

	return len(p), nil
}
