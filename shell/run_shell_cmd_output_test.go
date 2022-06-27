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

func testCommandOutputOrder(t *testing.T, withPtty bool) {
	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	// Specify a single (locking) buffer for both as a way to check that the output is being written in the correct
	// order
	var allOutputBuffer BufferWithLocking
	terragruntOptions.Writer = &allOutputBuffer
	terragruntOptions.ErrWriter = &allOutputBuffer

	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, "same")

	out, err := RunShellCommandWithOutput(terragruntOptions, "", false, false, "../testdata/test_outputs.sh", "same")

	require.NotNil(t, out, "Should get output")
	assert.Nil(t, err, "Should have no error")

	allOutputs := strings.Split(strings.TrimSpace(allOutputBuffer.String()), "\n")

	require.True(t, len(allOutputs) == 5, "Expected 5 entries, but got %d: %v", len(allOutputs), allOutputs)
	assert.Equal(t, "stdout1", allOutputs[0], "First one from stdout")
	assert.Equal(t, "stderr1", allOutputs[1], "First one from stderr")
	assert.Equal(t, "stdout2", allOutputs[2], "Second one from stdout")
	assert.Equal(t, "stderr2", allOutputs[3], "Second one from stderr")
	assert.Equal(t, "stderr3", allOutputs[4], "Third one from stderr")

	stdOutputs := strings.Split(strings.TrimSpace(out.Stdout), "\n")

	require.True(t, len(stdOutputs) == 2, "Expected 2 entries, but got %d: %v", len(stdOutputs), stdOutputs)
	assert.Equal(t, "stdout1", stdOutputs[0], "First one from stdout")
	assert.Equal(t, "stdout2", stdOutputs[1], "Second one from stdout")

	stdErrs := strings.Split(strings.TrimSpace(out.Stderr), "\n")

	require.True(t, len(stdErrs) == 3, "Expected 3 entries, but got %d: %v", len(stdErrs), stdErrs)
	assert.Equal(t, "stderr1", stdErrs[0], "First one from stderr")
	assert.Equal(t, "stderr2", stdErrs[1], "Second one from stderr")
	assert.Equal(t, "stderr3", stdErrs[2], "Second one from stderr")
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
