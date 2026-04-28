package a

import (
	"log"
	"testing"
	"testing/synctest"
	"time"

	stdtime "time"
)

// fakeTime is a stand-in for an imported third-party package that happens to
// expose a Sleep symbol. The analyzer must not confuse it with the stdlib
// time package.
type fakeTime struct{}

func (fakeTime) Sleep(time.Duration) {}

var notTime fakeTime

// helper is a local function used to wrap call sites for stack-walk coverage.
// Calls passed through helper exercise the "ancestor CallExpr whose Fun is a
// bare *ast.Ident, not a SelectorExpr" branch of the bubble walk.
func helper(f func()) { f() }

func TestInsideSynctest_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(time.Second)
		_ = time.After(time.Second)
		_ = time.Tick(time.Second)
		_ = time.NewTimer(time.Second)
		_ = time.NewTicker(time.Second)
		_ = time.AfterFunc(time.Second, func() {})
	})
}

func TestNestedSubtest_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Run("sub", func(t *testing.T) {
			time.Sleep(time.Second)
		})
	})
}

func TestGoroutineInsideBubble_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		go func() {
			time.Sleep(time.Second)
		}()
		synctest.Wait()
	})
}

func TestAliasedSynctestImport_OK(t *testing.T) {
	st := synctest.Test
	st(t, func(t *testing.T) {
		// Reassignment via local variable still preserves lexical containment
		// because the call expression `st(...)` does not match
		// `synctest.Test`. This therefore *does* get flagged — documenting
		// the limitation.
		time.Sleep(time.Second) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	})
}

func TestAliasedTimeImport_Bad(_ *testing.T) {
	stdtime.Sleep(stdtime.Second) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
}

func TestOutsideBubble_Bad(_ *testing.T) {
	time.Sleep(time.Second)                    // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	_ = time.After(time.Second)                // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
	_ = time.Tick(time.Second)                 // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Tick in tests`
	_ = time.NewTimer(time.Second)             // want `prefer testing/synctest \(synctest.Test\) over real-clock time.NewTimer in tests`
	_ = time.NewTicker(time.Second)            // want `prefer testing/synctest \(synctest.Test\) over real-clock time.NewTicker in tests`
	_ = time.AfterFunc(time.Second, func() {}) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.AfterFunc in tests`
}

func TestUnrelatedTimeCalls_OK(_ *testing.T) {
	_ = time.Now()
	_ = time.Since(time.Now())
	_ = time.Duration(1) * time.Second
}

// TestBareFunctionAncestor_Bad ensures the bubble walk handles ancestor
// CallExprs whose Fun is a plain *ast.Ident (not a SelectorExpr).
func TestBareFunctionAncestor_Bad(_ *testing.T) {
	helper(func() {
		time.Sleep(time.Second) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	})
}

// TestChainedSelectorAncestor_Bad ensures the bubble walk handles ancestor
// CallExprs whose Fun is a SelectorExpr but whose X is itself an expression
// rather than a bare Ident. log.Default() returns a *log.Logger, and the
// ancestor walk must skip it cleanly without panicking.
func TestChainedSelectorAncestor_Bad(_ *testing.T) {
	log.Default().Print(time.Now(), time.After(time.Second)) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
}

// TestNonSynctestPackageAncestor_Bad ensures the bubble walk skips ancestor
// calls whose selector resolves to a package other than testing/synctest.
func TestNonSynctestPackageAncestor_Bad(_ *testing.T) {
	log.Println(time.After(time.Second)) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
}

// TestNonTimePackageSelector_OK ensures a Sleep selector on something that
// merely shares the name with time.Sleep is not flagged.
func TestNonTimePackageSelector_OK(_ *testing.T) {
	notTime.Sleep(time.Second)
}

// TestChainedSelectorOnTime_OK exercises the timeFuncName early-return when
// the selector's X is not a bare Ident — `time.Now().Truncate(...)` has a
// SelectorExpr whose X is a CallExpr, so this must not be misread as time.X.
func TestChainedSelectorOnTime_OK(_ *testing.T) {
	_ = time.Now().Truncate(time.Second)
}

// TestBareCallOnLocalFunc_OK exercises the timeFuncName early-return for a
// CallExpr whose Fun is a bare Ident.
func TestBareCallOnLocalFunc_OK(_ *testing.T) {
	helper(func() {})
}
