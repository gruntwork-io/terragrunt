//go:build linux || darwin

package tf_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	FullOutput = []string{"stdout1", "stderr1", "stdout2", "stderr2", "stderr3"}
	Stdout     = []string{"stdout1", "stdout2"}
	Stderr     = []string{"stderr1", "stderr2", "stderr3"}
)

func TestCommandOutputPrefix(t *testing.T) {
	t.Parallel()

	prefix := "."
	terraformPath := "testdata/test_outputs.sh"

	prefixedOutput := make([]string, 0, len(FullOutput))
	for _, line := range FullOutput {
		prefixedOutput = append(prefixedOutput, fmt.Sprintf("prefix=%s tf-path=%s msg=%s", prefix, filepath.Base(terraformPath), line))
	}

	logFormatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())

	testCommandOutput(t, func(terragruntOptions *options.TerragruntOptions) {
		terragruntOptions.TFPath = terraformPath
	}, func(l log.Logger) log.Logger {
		l.SetOptions(log.WithFormatter(logFormatter))
		return l.WithField(placeholders.WorkDirKeyName, prefix)
	}, assertOutputs(t,
		prefixedOutput,
		Stdout,
		Stderr,
	))
}

// TestConcurrentSharedWriterNoRace reproduces the concurrent-write panic
// reported in os/exec's per-stream copy goroutines:
//
//	panic: runtime error: slice bounds out of range [:200] with capacity 144
//	bytes.(*Buffer).grow ... internal/writer.(*OriginalWriter).Write ...
//	logrus.(*Entry).write ... os/exec.(*Cmd).Start.func2
//
// os/exec.Cmd.Start drains stdout and stderr in separate goroutines, each with
// its own logrus.Logger (its own mutex). When Writers.Writer == Writers.ErrWriter
// both chains bottom out at the same io.Writer, which nothing serializes. Unlike
// TestCommandOutputPrefix, this uses a plain (unsynchronized) bytes.Buffer, as
// real callers do, so logTFOutput's syncWriter is the only guard.
//
// Run with -race; reverting the run_cmd.go fix makes this fail.
func TestConcurrentSharedWriterNoRace(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	var shared bytes.Buffer

	opts.Writers.Writer = &shared
	opts.Writers.ErrWriter = &shared
	opts.TerraformCliArgs.AppendArgument("apply")
	opts.TFPath = "testdata/test_concurrent_outputs.sh"

	out, err := tf.RunCommandWithOutput(t.Context(), logger.CreateLogger(), vexec.NewOSExec(), configbridge.TFRunOptsFromOpts(opts), "apply")
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Equal(t, 2000, strings.Count(out.Stdout.String(), "\n"), "stdout lines")
	assert.Equal(t, 2000, strings.Count(out.Stderr.String(), "\n"), "stderr lines")
}

// TestConcurrentHeadlessSharedWriterNoRace covers the headless branch of the fix.
// In headless mode buildOutWriter redirects stdout onto errWriter, so both stream
// loggers converge on ErrWriter; guarding ErrWriter alone therefore suffices.
//
// Run with -race; reverting the run_cmd.go fix makes this fail.
func TestConcurrentHeadlessSharedWriterNoRace(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	var stdoutSink, sharedStderr bytes.Buffer

	opts.Headless = true
	opts.Writers.Writer = &stdoutSink
	opts.Writers.ErrWriter = &sharedStderr
	opts.TerraformCliArgs.AppendArgument("apply")
	opts.TFPath = "testdata/test_concurrent_outputs.sh"

	out, err := tf.RunCommandWithOutput(t.Context(), logger.CreateLogger(), vexec.NewOSExec(), configbridge.TFRunOptsFromOpts(opts), "apply")
	require.NoError(t, err)
	require.NotNil(t, out)

	// -race is the real gate here: both stream loggers converge on the single
	// sharedStderr buffer. Assert no data was lost via the stable CmdOutput
	// capture (the shared buffer's line count is unstable because the log writer
	// splits lines on io.Copy chunk boundaries).
	require.NotZero(t, sharedStderr.Len())
	assert.Equal(t, 2000, strings.Count(out.Stdout.String(), "\n"), "stdout lines")
	assert.Equal(t, 2000, strings.Count(out.Stderr.String(), "\n"), "stderr lines")
}

func testCommandOutput(t *testing.T, withOptions func(*options.TerragruntOptions), withLogger func(log.Logger) log.Logger, assertResults func(string, *util.CmdOutput)) {
	t.Helper()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	// Specify a single (locking) buffer for both as a way to check that the output is being written in the correct
	// order
	var allOutputBuffer BufferWithLocking

	terragruntOptions.Writers.Writer = &allOutputBuffer
	terragruntOptions.Writers.ErrWriter = &allOutputBuffer

	terragruntOptions.TerraformCliArgs.AppendArgument("same")
	terragruntOptions.TFPath = "testdata/test_outputs.sh"

	withOptions(terragruntOptions)

	l := logger.CreateLogger()
	l = withLogger(l)

	out, err := tf.RunCommandWithOutput(t.Context(), l, vexec.NewOSExec(), configbridge.TFRunOptsFromOpts(terragruntOptions), "same")

	assert.NotNil(t, out, "Should get output")
	require.NoError(t, err, "Should have no error")

	assert.NotNil(t, out, "Should get output")
	assertResults(allOutputBuffer.String(), out)
}

func assertOutputs(
	t *testing.T,
	expectedAllOutputs []string,
	expectedStdOutputs []string,
	expectedStdErrs []string,
) func(string, *util.CmdOutput) {
	t.Helper()

	return func(allOutput string, out *util.CmdOutput) {
		allOutputs := strings.Split(strings.TrimSpace(allOutput), "\n")
		assert.Len(t, allOutputs, len(expectedAllOutputs))

		for i := range allOutputs {
			assert.Contains(t, allOutputs[i], expectedAllOutputs[i], allOutputs[i])
		}

		stdOutputs := strings.Split(strings.TrimSpace(out.Stdout.String()), "\n")
		assert.Equal(t, expectedStdOutputs, stdOutputs)

		stdErrs := strings.Split(strings.TrimSpace(out.Stderr.String()), "\n")
		assert.Equal(t, expectedStdErrs, stdErrs)
	}
}

// A goroutine-safe bytes.Buffer
type BufferWithLocking struct {
	buffer bytes.Buffer
	mutex  sync.Mutex
}

// Write appends the contents of p to the buffer, growing the buffer as needed. It returns
// the number of bytes written.
func (s *BufferWithLocking) Write(p []byte) (n int, err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.buffer.Write(p)
}

// String returns the contents of the unread portion of the buffer
// as a string.  If the Buffer is a nil pointer, it returns "<nil>".
func (s *BufferWithLocking) String() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.buffer.String()
}
