//go:build linux || darwin
// +build linux darwin

package shell

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestCommandOutputOrder(t *testing.T) {
	t.Parallel()

	t.Run("withPtty", func(t *testing.T) {
		t.Parallel()
		testCommandOutputOrder(t, true)
	})
	t.Run("withoutPtty", func(t *testing.T) {
		t.Parallel()
		testCommandOutputOrder(t, false)
	})
}

func noop[T any](t T) {}

var FULL_OUTPUT = []string{"stdout1", "stderr1", "stdout2", "stderr2", "stderr3"}
var STDOUT = []string{"stdout1", "stdout2"}
var STDERR = []string{"stderr1", "stderr2", "stderr3"}

func testCommandOutputOrder(t *testing.T, withPtty bool) {
	testCommandOutput(t, noop[*options.TerragruntOptions], assertOutputs(t, FULL_OUTPUT, STDOUT, STDERR))
}

func TestCommandOutputPrefix(t *testing.T) {
	prefix := "PREFIX> "
	prefixedOutput := []string{}
	for _, line := range FULL_OUTPUT {
		prefixedOutput = append(prefixedOutput, prefix+line)
	}
	testCommandOutput(t, func(terragruntOptions *options.TerragruntOptions) {
		terragruntOptions.IncludeModulePrefix = true
		terragruntOptions.OutputPrefix = prefix
	}, assertOutputs(t,
		prefixedOutput,
		STDOUT,
		STDERR,
	))
}

func testCommandOutput(t *testing.T, withOptions func(*options.TerragruntOptions), assertResults func(string, *CmdOutput)) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	// Specify a single (locking) buffer for both as a way to check that the output is being written in the correct
	// order
	var allOutputBuffer BufferWithLocking
	terragruntOptions.Writer = &allOutputBuffer
	terragruntOptions.ErrWriter = &allOutputBuffer

	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "same")

	withOptions(terragruntOptions)

	out, err := RunShellCommandWithOutput(terragruntOptions, "", false, false, "../testdata/test_outputs.sh", "same")

	require.NotNil(t, out, "Should get output")
	assert.Nil(t, err, "Should have no error")

	assertResults(allOutputBuffer.String(), out)
}

func assertOutputs(
	t *testing.T,
	expectedAllOutputs []string,
	expectedStdOutputs []string,
	expectedStdErrs []string,
) func(string, *CmdOutput) {
	return func(allOutput string, out *CmdOutput) {
		allOutputs := strings.Split(strings.TrimSpace(allOutput), "\n")
		assert.Equal(t, expectedAllOutputs, allOutputs)

		stdOutputs := strings.Split(strings.TrimSpace(out.Stdout), "\n")
		assert.Equal(t, expectedStdOutputs, stdOutputs)

		stdErrs := strings.Split(strings.TrimSpace(out.Stderr), "\n")
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
