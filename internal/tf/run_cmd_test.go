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
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	Stdout = []string{"stdout1", "stdout2"}
	Stderr = []string{"stderr1", "stderr2", "stderr3"}
)

func TestCommandOutputPrefix(t *testing.T) {
	t.Parallel()

	prefix := "."
	terraformPath := "testdata/test_outputs.sh"

	prefixed := func(lines []string) []string {
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			out = append(
				out,
				fmt.Sprintf("prefix=%s tf-path=%s msg=%s", prefix, filepath.Base(terraformPath), line),
			)
		}

		return out
	}

	logFormatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())

	testCommandOutput(t, func(terragruntOptions *options.TerragruntOptions) {
		terragruntOptions.TFPath = terraformPath
	}, func(l log.Logger) log.Logger {
		l.SetOptions(log.WithFormatter(logFormatter))
		return l.WithField(placeholders.WorkDirKeyName, prefix)
	}, assertOutputs(t,
		prefixed(Stdout),
		prefixed(Stderr),
		Stdout,
		Stderr,
	))
}

func testCommandOutput(
	t *testing.T,
	withOptions func(*options.TerragruntOptions),
	withLogger func(log.Logger) log.Logger,
	assertResults func(string, *util.CmdOutput),
) {
	t.Helper()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	// Specify a single (locking) buffer for both as a way to check that the output is being written in the correct
	// order
	var allOutputBuffer BufferWithLocking

	terragruntOptions.TerraformCliArgs.AppendArgument("same")
	terragruntOptions.TFPath = "testdata/test_outputs.sh"

	withOptions(terragruntOptions)

	l := logger.CreateLogger()
	l = withLogger(l)

	v := venvtest.New().
		WithExec(vexec.NewOSExec()).
		WithWriter(&allOutputBuffer).
		WithErrWriter(&allOutputBuffer)

	out, err := tf.RunCommandWithOutput(
		t.Context(),
		l,
		v,
		configbridge.TFRunOptsFromOpts(terragruntOptions),
		"same",
	)

	assert.NotNil(t, out, "Should get output")
	require.NoError(t, err, "Should have no error")

	assert.NotNil(t, out, "Should get output")
	assertResults(allOutputBuffer.String(), out)
}

func assertOutputs(
	t *testing.T,
	expectedMergedStdout []string,
	expectedMergedStderr []string,
	expectedStdOutputs []string,
	expectedStdErrs []string,
) func(string, *util.CmdOutput) {
	t.Helper()

	return func(allOutput string, out *util.CmdOutput) {
		allOutputs := strings.Split(strings.TrimSpace(allOutput), "\n")
		assert.Len(t, allOutputs, len(expectedMergedStdout)+len(expectedMergedStderr))

		// Cross-stream arrival order in the merged buffer is scheduler-dependent, so only per-stream order is asserted.
		assertContainsInOrder(t, allOutputs, expectedMergedStdout)
		assertContainsInOrder(t, allOutputs, expectedMergedStderr)

		stdOutputs := strings.Split(strings.TrimSpace(out.Stdout.String()), "\n")
		assert.Equal(t, expectedStdOutputs, stdOutputs)

		stdErrs := strings.Split(strings.TrimSpace(out.Stderr.String()), "\n")
		assert.Equal(t, expectedStdErrs, stdErrs)
	}
}

// assertContainsInOrder asserts that each expected entry matches a merged output line in the given relative order.
func assertContainsInOrder(t *testing.T, lines []string, expected []string) {
	t.Helper()

	matched := 0

	for _, line := range lines {
		if matched < len(expected) && strings.Contains(line, expected[matched]) {
			matched++
		}
	}

	assert.Equal(t, len(expected), matched, "merged output must contain %v in per-stream order", expected)
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
