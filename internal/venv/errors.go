package venv

import "errors"

// ErrVenvEnvUnset is the panic value [Venv.RequireEnv] raises when Env is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Env rather than a runtime condition.
var ErrVenvEnvUnset = errors.New("venv.Venv.Env is required but unset")

// ErrVenvEnvNil is the panic value [Venv.WithEnv] raises when its env
// argument is nil. Every caller builds the map it passes, so a nil points
// at a caller bug rather than a runtime condition.
var ErrVenvEnvNil = errors.New("venv.Venv.WithEnv: env must not be nil")

// ErrVenvFSUnset is the panic value [Venv.RequireFS] raises when FS is nil.
// Production callers build the Venv through [OSVenv], so it points at a test
// that forgot to set FS rather than a runtime condition.
var ErrVenvFSUnset = errors.New("venv.Venv.FS is required but unset")

// ErrVenvExecUnset is the panic value [Venv.RequireExec] raises when Exec is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Exec rather than a runtime condition.
var ErrVenvExecUnset = errors.New("venv.Venv.Exec is required but unset")

// ErrVenvHTTPUnset is the panic value [Venv.RequireHTTP] raises when HTTP is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set HTTP rather than a runtime condition.
var ErrVenvHTTPUnset = errors.New("venv.Venv.HTTP is required but unset")

// ErrVenvBrowserUnset is the panic value [Venv.RequireBrowser] raises when
// Browser is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Browser rather than a runtime condition.
var ErrVenvBrowserUnset = errors.New("venv.Venv.Browser is required but unset")

// ErrVenvStdinUnset is the panic value [Venv.RequireStdin] raises when Stdin is
// nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Stdin rather than a runtime condition.
var ErrVenvStdinUnset = errors.New("venv.Venv.Stdin is required but unset")

// ErrVenvTerminalUnset is the panic value [Venv.RequireTerminal] raises when
// Terminal is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Terminal rather than a runtime condition.
var ErrVenvTerminalUnset = errors.New("venv.Venv.Terminal is required but unset")

// ErrVenvWritersUnset is the panic value [Venv.RequireWriters] raises when
// Writers, or either writer it carries, is nil. Production callers build the
// Venv through [OSVenv], so it points at a test that forgot to set Writers
// rather than a runtime condition.
var ErrVenvWritersUnset = errors.New("venv.Venv.Writers is required but unset")

// ErrVenvListenUnset is the panic value [Venv.RequireListen] raises when Listen
// is nil. Production callers build the Venv through [OSVenv], so it points at a
// test that forgot to set Listen rather than a runtime condition.
var ErrVenvListenUnset = errors.New("venv.Venv.Listen is required but unset")

// ErrVenvSignalsUnset is the panic value [Venv.RequireSignals] raises when
// Signals is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Signals rather than a runtime condition.
var ErrVenvSignalsUnset = errors.New("venv.Venv.Signals is required but unset")

// ErrVenvPlatformUnset is the panic value [Venv.RequirePlatform] raises when
// Platform is nil. Production callers build the Venv through [OSVenv], so it
// points at a test that forgot to set Platform rather than a runtime condition.
var ErrVenvPlatformUnset = errors.New("venv.Venv.Platform is required but unset")

// ErrVenvGOOSUnset is the panic value [Venv.RequireGOOS] raises when GOOS is empty.
var ErrVenvGOOSUnset = errors.New("venv.Venv.Platform.GOOS is required but unset")

// ErrVenvGOARCHUnset is the panic value [Venv.RequireGOARCH] raises when GOARCH is empty.
var ErrVenvGOARCHUnset = errors.New("venv.Venv.Platform.GOARCH is required but unset")

// ErrVenvUserHomeDirUnset is the panic value [Venv.RequireUserHomeDir] raises
// when UserHomeDir is nil.
var ErrVenvUserHomeDirUnset = errors.New("venv.Venv.Platform.UserHomeDir is required but unset")

// ErrVenvUserCacheDirUnset is the panic value [Venv.RequireUserCacheDir] raises
// when UserCacheDir is nil.
var ErrVenvUserCacheDirUnset = errors.New("venv.Venv.Platform.UserCacheDir is required but unset")

// ErrVenvUserConfigDirUnset is the panic value [Venv.RequireUserConfigDir]
// raises when UserConfigDir is nil.
var ErrVenvUserConfigDirUnset = errors.New("venv.Venv.Platform.UserConfigDir is required but unset")

// ErrVenvTempDirUnset is the panic value [Venv.RequireTempDir] raises when
// TempDir is nil.
var ErrVenvTempDirUnset = errors.New("venv.Venv.Platform.TempDir is required but unset")

// ErrVenvReplaceEnvironUnset is the panic value [Venv.RequireReplaceEnviron]
// raises when ReplaceEnviron is nil.
var ErrVenvReplaceEnvironUnset = errors.New("venv.Venv.Platform.ReplaceEnviron is required but unset")
