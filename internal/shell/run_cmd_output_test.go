//go:build linux || darwin

package shell_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

func TestCommandOutputOrder(t *testing.T) {
	t.Parallel()

	t.Run("withPtty", func(t *testing.T) {
		t.Parallel()
		testCommandOutputOrder(t, true,
			[]string{"stdout1", "stdout2"},
			[]string{"stderr1", "stderr2", "stderr3"},
			[]string{"stdout1", "stdout2"},
			[]string{"stderr1", "stderr2", "stderr3"},
		)
	})
	t.Run("withoutPtty", func(t *testing.T) {
		t.Parallel()
		testCommandOutputOrder(t, false,
			nil,
			[]string{"stderr1", "stderr2", "stderr3"},
			[]string{"stdout1", "stdout2"},
			[]string{"stderr1", "stderr2", "stderr3"},
		)
	})
}

func noop[T any](t T) {}

func testCommandOutputOrder(
	t *testing.T,
	withPtty bool,
	mergedStdout []string,
	mergedStderr []string,
	stdout []string,
	stderr []string,
) {
	t.Helper()

	testCommandOutput(
		t,
		noop[*options.TerragruntOptions],
		assertOutputs(t, mergedStdout, mergedStderr, stdout, stderr),
		withPtty,
	)
}

func testCommandOutput(
	t *testing.T,
	withOptions func(*options.TerragruntOptions),
	assertResults func(string, *util.CmdOutput),
	allocateStdout bool,
) {
	t.Helper()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	// Specify a single (locking) buffer for both as a way to check that the output is being written in the correct
	// order
	var allOutputBuffer BufferWithLocking

	terragruntOptions.TerraformCliArgs.AppendArgument("same")

	withOptions(terragruntOptions)

	l := logger.CreateLogger()

	v := venvtest.New().
		WithExec(vexec.NewOSExec()).
		WithWriter(&allOutputBuffer).
		WithErrWriter(&allOutputBuffer)

	out, err := shell.RunCommandWithOutput(
		t.Context(),
		l,
		&v,
		configbridge.ShellRunOptsFromOpts(terragruntOptions),
		"",
		!allocateStdout,
		false,
		"testdata/test_outputs.sh",
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
