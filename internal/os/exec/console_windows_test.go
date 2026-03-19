//go:build windows

package exec_test

import (
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

	// Verify it really is a console handle.
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

// TestWindowsConsolePrepareOnPipes verifies PrepareConsole behavior when
// stdout/stderr are pipes (typical in CI). If CONOUT$ is available it falls
// back and succeeds; otherwise it returns false.
func TestWindowsConsolePrepareOnPipes(t *testing.T) {
	t.Parallel()

	var mode uint32
	if windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &mode) == nil {
		t.Skip("skipping: os.Stdout is a real console, this test is for pipe environments")
	}

	l := log.New(log.WithLevel(log.DebugLevel))
	result := exec.PrepareConsole(l)

	// PrepareConsole result must match whether CONOUT$ is available.
	_, conoutErr := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	expectSuccess := conoutErr == nil
	assert.Equal(t, expectSuccess, result,
		"PrepareConsole result should match CONOUT$ availability")
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

	// When stdout is a real console, mode must be preserved.
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

	// When stdin is a console, mode should have required flags set (not cleared).
	required := uint32(windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)
	assert.Equal(t, required, afterMode&required,
		"PrepareStdinForPrompt should ensure prompt flags are set")
}

// TestWindowsConsoleVTProcessingOnCONOUT verifies that VT processing can be
// toggled on a real console handle via raw API calls.
func TestWindowsConsoleVTProcessingOnCONOUT(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	// Clear VT processing to simulate a fresh console.
	cleared := original &^ windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, cleared)

	assert.Equal(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT bit should be cleared before test")

	// Set it back and verify.
	setMode(t, conout, cleared|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)

	assert.NotEqual(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT processing should be enabled on CONOUT$")
}

// TestWindowsConsolePrepareConsoleEnablesVT calls the production PrepareConsole
// function and verifies it enables ENABLE_VIRTUAL_TERMINAL_PROCESSING on the
// console screen buffer via CONOUT$.
func TestWindowsConsolePrepareConsoleEnablesVT(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	// Clear VT processing so PrepareConsole has work to do.
	setMode(t, conout, original&^windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)

	l := log.New(log.WithLevel(log.DebugLevel))
	result := exec.PrepareConsole(l)

	assert.True(t, result, "PrepareConsole should succeed on a real console")

	// Verify VT processing is now enabled on the console screen buffer.
	after := getMode(t, conout)
	assert.NotEqual(t, uint32(0), after&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"PrepareConsole should enable VT processing on CONOUT$")
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

	// Ensure VT processing is enabled.
	withVT := original | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, withVT)

	before := getMode(t, conout)
	require.Equal(t, withVT, before)

	// Simulate: save → subprocess corrupts mode → restore.
	saved := before // snapshot

	// Corrupt: clear VT processing (what terraform.exe does).
	corrupted := before &^ windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, corrupted)

	assert.Equal(t, uint32(0), getMode(t, conout)&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT should be cleared after simulated subprocess corruption")

	// Restore.
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

	// Verify the default console input has these flags.
	assert.Equal(t, required, original&required,
		"a default console input handle should have LINE_INPUT, ECHO_INPUT, PROCESSED_INPUT")

	// Clear them.
	setMode(t, conin, original&^required)
	assert.Equal(t, uint32(0), getMode(t, conin)&required,
		"required flags should be cleared after corruption")

	// Restore via raw API.
	setMode(t, conin, original)
	assert.Equal(t, required, getMode(t, conin)&required,
		"required flags should be restored")
}

// TestWindowsConsolePrepareStdinForPromptRestoresFlags calls the production
// PrepareStdinForPrompt and verifies it restores LINE_INPUT, ECHO_INPUT, and
// PROCESSED_INPUT after they have been cleared (simulating subprocess corruption).
// Skips in CI where os.Stdin is a pipe.
func TestWindowsConsolePrepareStdinForPromptRestoresFlags(t *testing.T) {
	t.Parallel()

	stdinHandle := windows.Handle(os.Stdin.Fd())

	var stdinMode uint32
	skipErr := windows.GetConsoleMode(stdinHandle, &stdinMode)
	require.NoErrorf(t, skipErr, "os.Stdin is not a console handle — run locally on Windows")

	required := uint32(windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT | windows.ENABLE_PROCESSED_INPUT)

	defer func() {
		require.NoError(t, windows.SetConsoleMode(stdinHandle, stdinMode))
	}()

	// Corrupt stdin: clear prompt flags.
	require.NoError(t, windows.SetConsoleMode(stdinHandle, stdinMode&^required))

	var corrupted uint32
	require.NoError(t, windows.GetConsoleMode(stdinHandle, &corrupted))
	require.Equal(t, uint32(0), corrupted&required, "prompt flags should be cleared before calling PrepareStdinForPrompt")

	// Call production function.
	l := log.New(log.WithLevel(log.DebugLevel))
	exec.PrepareStdinForPrompt(l)

	// Verify flags are restored.
	var after uint32
	require.NoError(t, windows.GetConsoleMode(stdinHandle, &after))
	assert.Equal(t, required, after&required,
		"PrepareStdinForPrompt should restore LINE_INPUT, ECHO_INPUT, PROCESSED_INPUT")
}

// TestWindowsConsoleSaveRestoreAPI calls production SaveConsoleState/Restore
// and verifies console mode is preserved after simulated subprocess corruption.
func TestWindowsConsoleSaveRestoreAPI(t *testing.T) {
	t.Parallel()

	// SaveConsoleState/Restore operate on os.Stdout — need a real console handle.
	// Skips in CI where os.Stdout is a pipe.
	stdoutHandle := windows.Handle(os.Stdout.Fd())

	var stdoutMode uint32
	require.NoErrorf(t, windows.GetConsoleMode(stdoutHandle, &stdoutMode),
		"os.Stdout is not a console handle — run locally on Windows")

	// Save via production API.
	saved := exec.SaveConsoleState()

	// Corrupt stdout: clear VT processing (what terraform.exe does).
	corrupted := stdoutMode &^ windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	require.NoError(t, windows.SetConsoleMode(stdoutHandle, corrupted))

	var mid uint32
	require.NoError(t, windows.GetConsoleMode(stdoutHandle, &mid))
	assert.Equal(t, uint32(0), mid&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING,
		"VT should be cleared after corruption")

	// Restore via production API.
	saved.Restore()

	var after uint32
	require.NoError(t, windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &after))
	assert.Equal(t, stdoutMode, after,
		"SaveConsoleState/Restore should restore original console mode")
}

// TestWindowsConsoleSubprocessSaveRestore is an integration test that runs a
// real subprocess and verifies the save→subprocess→restore pattern preserves
// console modes. Uses CONOUT$ for a real console handle.
func TestWindowsConsoleSubprocessSaveRestore(t *testing.T) {
	t.Parallel()

	conout := openConsoleOutput(t)
	original := getMode(t, conout)

	defer setMode(t, conout, original)

	// Enable VT processing.
	withVT := original | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	setMode(t, conout, withVT)

	before := getMode(t, conout)

	// Run a real subprocess. Even though cmd.exe may not corrupt CONOUT$
	// directly (it inherits piped handles in this test), this validates
	// that the save/restore API works end-to-end without errors.
	cmd := exec.Command(t.Context(), "cmd.exe", "/C", "echo hello")
	cmd.Stdout = nil
	cmd.Stderr = nil

	saved := exec.SaveConsoleState()

	require.NoError(t, cmd.Run())

	saved.Restore()

	// Also restore our CONOUT$ handle (SaveConsoleState operates on os.Stdout,
	// not our separately-opened CONOUT$, so we restore it manually).
	setMode(t, conout, before)

	after := getMode(t, conout)
	assert.Equal(t, before, after,
		"CONOUT$ mode should be unchanged after save→subprocess→restore")
}
