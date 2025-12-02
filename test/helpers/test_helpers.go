package helpers

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const defaultDirPerms = 0755

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func ValidateHookTraceParent(t *testing.T, hook, str string) {
	t.Helper()

	traceparentLine := ""

	for line := range strings.SplitSeq(str, "\n") {
		if strings.HasPrefix(line, hook+" {\"traceparent\": \"") {
			traceparentLine = line
			break
		}
	}

	require.NotEmpty(t, traceparentLine, "Expected "+hook+" output with traceparent value")
	re := regexp.MustCompile(hook + ` \{"traceparent": "([^"]+)"\}`)
	matches := re.FindStringSubmatch(traceparentLine)

	const matchesCount = 2

	require.Len(t, matches, matchesCount, "Expected to extract traceparent value from hook output")

	traceparentValue := matches[1]
	require.NotEmpty(t, traceparentValue, "Traceparent value should not be empty")
	require.Regexp(t, `^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`, traceparentValue, "Traceparent value should match W3C traceparent format")
}

// CreateFile creates an empty file at the given path, creating parent directories if needed.
func CreateFile(t *testing.T, paths ...string) {
	t.Helper()

	fullPath := filepath.Join(paths...)

	err := os.MkdirAll(filepath.Dir(fullPath), defaultDirPerms)
	require.NoError(t, err)

	f, err := os.Create(fullPath)
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}

// CreateGitRepo initializes a git repository at the given path and creates an initial commit.
func CreateGitRepo(t *testing.T, path string) {
	t.Helper()

	ctx := t.Context()

	// Initialize git repo
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = path

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init failed: %s", string(output))

	// Configure user for the repo
	cmd = exec.CommandContext(ctx, "git", "config", "user.email", "test@test.com")
	cmd.Dir = path
	_, err = cmd.CombinedOutput()
	require.NoError(t, err, "git config user.email failed")

	cmd = exec.CommandContext(ctx, "git", "config", "user.name", "Test User")
	cmd.Dir = path
	_, err = cmd.CombinedOutput()
	require.NoError(t, err, "git config user.name failed")

	// Add all files and commit
	cmd = exec.CommandContext(ctx, "git", "add", "-A")
	cmd.Dir = path
	_, err = cmd.CombinedOutput()
	require.NoError(t, err, "git add failed")

	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "initial commit", "--allow-empty")
	cmd.Dir = path
	_, err = cmd.CombinedOutput()
	require.NoError(t, err, "git commit failed")
}

// IsExperimentMode returns true if the TG_EXPERIMENT_MODE environment variable is set.
func IsExperimentMode(t *testing.T) bool {
	t.Helper()
	// Enable only on explicit true
	val := strings.TrimSpace(os.Getenv("TG_EXPERIMENT_MODE"))

	return strings.EqualFold(val, "true")
}

// ExecWithTestLogger executes a command and logs the output to the test logger.
func ExecWithTestLogger(t *testing.T, dir, command string, args ...string) {
	t.Helper()

	ctx := t.Context()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer

	prefix := strings.Join(append([]string{command}, args...), " ")

	stdoutLogger := &testLogger{t: t, prefix: prefix + " stdout"}
	stderrLogger := &testLogger{t: t, prefix: prefix + " stderr"}

	cmd.Stdout = io.MultiWriter(&stdout, stdoutLogger)
	cmd.Stderr = io.MultiWriter(&stderr, stderrLogger)

	err := cmd.Run()
	if err != nil {
		t.Logf("Command failed: %s %v", command, args)
		t.Logf("Full stdout:\n%s", stdout.String())
		t.Logf("Full stderr:\n%s", stderr.String())
	}

	require.NoError(t, err)
}

// PointerTo returns a pointer to the given parameter.
// Useful for constructing pointers to primitive types in test tables, etc.
func PointerTo[T any](v T) *T {
	return &v
}

type testLogger struct {
	t      *testing.T
	prefix string
	buffer bytes.Buffer
}

func (tl *testLogger) Write(p []byte) (n int, err error) {
	n = len(p)
	tl.buffer.Write(p)

	for {
		line, err := tl.buffer.ReadBytes('\n')
		if err != nil {
			tl.buffer.Write(line)
			break
		}

		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}

		if len(line) > 0 {
			tl.t.Logf("[%s] %s", tl.prefix, string(line))
		}
	}

	//nolint:nilerr
	return n, nil
}

// ExecWithMiseAndTestLogger executes a command using mise and logs the output to the test logger.
func ExecWithMiseAndTestLogger(t *testing.T, dir, command string, args ...string) {
	t.Helper()

	tool := determineToolName(command)

	args = append([]string{"x", tool, "--", command}, args...)

	ExecWithTestLogger(t, dir, "mise", args...)
}

// ExecAndCaptureOutput executes a command and captures the stdout and stderr.
func ExecAndCaptureOutput(t *testing.T, dir, command string, args ...string) (string, string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), command, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Start()
	require.NoError(t, err)

	err = cmd.Wait()
	require.NoError(t, err)

	return stdout.String(), stderr.String()
}

// ExecWithMiseAndCaptureOutput executes a command using mise and captures the stdout and stderr.
// This is useful for commands that are being tested as installed via mise, as it doesn't depend
// on the PATH being set correctly.
func ExecWithMiseAndCaptureOutput(t *testing.T, dir, command string, args ...string) (string, string) {
	t.Helper()

	tool := determineToolName(command)

	args = append([]string{"x", tool, "--", command}, args...)

	return ExecAndCaptureOutput(t, dir, "mise", args...)
}

// determineToolName determines the tool name to use for the given command.
func determineToolName(command string) string {
	switch command {
	case "tofu":
		return "opentofu"
	case "npm":
		return "node"
	}

	return command
}
