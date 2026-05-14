//go:build windows

package exec_test

import (
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// openConsoleOutput opens CONOUT$ directly, which always returns a real
// console handle even when os.Stdin/os.Stdout are pipes (e.g. in GitHub Actions).
// Skips the test if no console is attached at all.
func openConsoleOutput(t *testing.T) *os.File {
	t.Helper()

	f, err := os.OpenFile("CONOUT$", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("skipping: no console attached (CONOUT$ unavailable): %v", err)
	}

	t.Cleanup(func() { f.Close() })

	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		f.Close()
		t.Skipf("skipping: CONOUT$ is not a usable console handle: %v", err)
	}

	return f
}

func openConsoleInput(t *testing.T) *os.File {
	t.Helper()

	f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("skipping: no console attached (CONIN$ unavailable): %v", err)
	}

	t.Cleanup(func() { f.Close() })

	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		f.Close()
		t.Skipf("skipping: CONIN$ is not a usable console handle: %v", err)
	}

	return f
}

func getMode(t *testing.T, f *os.File) uint32 {
	t.Helper()

	var mode uint32
	require.NoError(t, windows.GetConsoleMode(windows.Handle(f.Fd()), &mode))

	return mode
}

func setMode(t *testing.T, f *os.File, mode uint32) {
	t.Helper()
	require.NoError(t, windows.SetConsoleMode(windows.Handle(f.Fd()), mode))
}

// TestWindowsConsoleStateOnPipes verifies that SaveConsoleState and Restore
// work without error when standard handles are pipes (CI). The saved state
// should round-trip: save then restore should not change the console mode.
func TestWindowsConsoleStateOnPipes(t *testing.T) {
	t.Parallel()

	var beforeMode uint32
	stdoutIsConsole := windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &beforeMode) == nil

	saved := exec.SaveConsoleState()
	saved.Restore()

	var afterMode uint32
	afterIsConsole := windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &afterMode) == nil

	assert.Equal(t, stdoutIsConsole, afterIsConsole,
		"stdout console status should not change after save/restore")

	assert.Equal(t, beforeMode, afterMode,
		"stdout console mode should be unchanged after save/restore")
}

// TestWindowsConsolePrepareStdinOnPipes verifies PrepareStdinForPrompt handles
// pipe stdin gracefully and does not corrupt console mode.
func TestWindowsConsolePrepareStdinOnPipes(t *testing.T) {
	t.Parallel()

	var beforeMode uint32
	stdinIsConsole := windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &beforeMode) == nil

	l := log.New(log.WithLevel(log.DebugLevel))
	exec.PrepareStdinForPrompt(l)

	var afterMode uint32
	afterIsConsole := windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &afterMode) == nil

	assert.Equal(t, stdinIsConsole, afterIsConsole,
		"stdin console status should not change after PrepareStdinForPrompt")
	assert.Equal(t, beforeMode, afterMode,
		"stdin console mode should be unchanged after PrepareStdinForPrompt on pipes")
}

// TestWindowsConsoleVTProcessingOnCONOUT verifies that VT processing can be
// toggled on a real console handle via raw API calls.
func TestWindowsConsoleVTProcessingOnCONOUT(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	cleared := original &^ windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, cleared)

	assert.Equal(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT bit should be cleared before test")

	setMode(t, conout, cleared|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)

	assert.NotEqual(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT processing should be enabled on CONOUT$")
}

// TestWindowsConsoleSaveRestoreOnCONOUT verifies the full save→corrupt→restore
// cycle using a real console handle from CONOUT$. This is the core regression
// test: subprocesses like "terraform version" clear VT processing, and Restore
// must bring it back.
func TestWindowsConsoleSaveRestoreOnCONOUT(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	withVT := original | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, withVT)

	before := getMode(t, conout)
	require.Equal(t, withVT, before)

	saved := before

	corrupted := before &^ windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, corrupted)

	assert.Equal(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT should be cleared after simulated subprocess corruption")

	setMode(t, conout, saved)

	after := getMode(t, conout)
	assert.Equal(t, before, after,
		"console mode must be identical after save→corrupt→restore cycle")
}

// TestWindowsConsoleStdinFlagsOnCONIN verifies stdin prompt flags can be
// cleared and restored via raw API on a real console input handle.
func TestWindowsConsoleStdinFlagsOnCONIN(t *testing.T) {
	t.Parallel()

	conin := openConsoleInput(t)
	original := getMode(t, conin)

	defer setMode(t, conin, original)

	required := uint32(windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)

	assert.Equal(t, required, original&required,
		"a default console input handle should have LINE_INPUT, ECHO_INPUT, PROCESSED_INPUT")

	setMode(t, conin, original&^required)
	assert.Equal(t, uint32(0), getMode(t, conin)&required,
		"required flags should be cleared after corruption")

	setMode(t, conin, original)
	assert.Equal(t, required, getMode(t, conin)&required,
		"required flags should be restored")
}

// TestWindowsConsoleSubprocessSaveRestore is an integration test that runs a
// real subprocess and verifies the save→subprocess→restore pattern preserves
// console modes. Uses CONOUT$ for a real console handle.
func TestWindowsConsoleSubprocessSaveRestore(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	withVT := original | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, withVT)

	before := getMode(t, conout)

	cmd := exec.Command(t.Context(), vexec.NewOSExec(), "cmd.exe", "/C", "echo hello")
	cmd.SetStdout(nil)
	cmd.SetStderr(nil)

	saved := exec.SaveConsoleState()

	require.NoError(t, cmd.Run(logger.CreateLogger()))

	saved.Restore()

	setMode(t, conout, before)

	after := getMode(t, conout)
	assert.Equal(t, before, after,
		"CONOUT$ mode should be unchanged after save→subprocess→restore")
}
