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
		time.Sleep(0)
		_ = time.After(0)
		_ = time.Tick(0)
		_ = time.NewTimer(0)
		_ = time.NewTicker(0)
		_ = time.AfterFunc(0, func() {})
	})
}

func TestNestedSubtest_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		t.Run("sub", func(t *testing.T) {
			time.Sleep(0)
		})
	})
}

func TestGoroutineInsideBubble_OK(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		go func() {
			time.Sleep(0)
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
		time.Sleep(0) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	})
}

func TestAliasedTimeImport_Bad(_ *testing.T) {
	stdtime.Sleep(0) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
}

func TestOutsideBubble_Bad(_ *testing.T) {
	time.Sleep(0)                    // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	_ = time.After(0)                // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
	_ = time.Tick(0)                 // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Tick in tests`
	_ = time.NewTimer(0)             // want `prefer testing/synctest \(synctest.Test\) over real-clock time.NewTimer in tests`
	_ = time.NewTicker(0)            // want `prefer testing/synctest \(synctest.Test\) over real-clock time.NewTicker in tests`
	_ = time.AfterFunc(0, func() {}) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.AfterFunc in tests`
}

func TestUnrelatedTimeCalls_OK(_ *testing.T) {
	_ = time.Now()
	_ = time.Since(time.Now())
	_ = time.Duration(1) * 0
}

// TestBareFunctionAncestor_Bad ensures the bubble walk handles ancestor
// CallExprs whose Fun is a plain *ast.Ident (not a SelectorExpr).
func TestBareFunctionAncestor_Bad(_ *testing.T) {
	helper(func() {
		time.Sleep(0) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
	})
}

// TestChainedSelectorAncestor_Bad ensures the bubble walk handles ancestor
// CallExprs whose Fun is a SelectorExpr but whose X is itself an expression
// rather than a bare Ident. log.Default() returns a *log.Logger, and the
// ancestor walk must skip it cleanly without panicking.
func TestChainedSelectorAncestor_Bad(_ *testing.T) {
	log.Default().Print(time.Now(), time.After(0)) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
}

// TestNonSynctestPackageAncestor_Bad ensures the bubble walk skips ancestor
// calls whose selector resolves to a package other than testing/synctest.
func TestNonSynctestPackageAncestor_Bad(_ *testing.T) {
	log.Println(time.After(0)) // want `prefer testing/synctest \(synctest.Test\) over real-clock time.After in tests`
}

// TestNonTimePackageSelector_OK ensures a Sleep selector on something that
// merely shares the name with time.Sleep is not flagged.
func TestNonTimePackageSelector_OK(_ *testing.T) {
	notTime.Sleep(0)
}

// TestChainedSelectorOnTime_OK exercises the timeFuncName early-return when
// the selector's X is not a bare Ident — `time.Now().Truncate(...)` has a
// SelectorExpr whose X is a CallExpr, so this must not be misread as time.X.
func TestChainedSelectorOnTime_OK(_ *testing.T) {
	_ = time.Now().Truncate(0)
}

// TestBareCallOnLocalFunc_OK exercises the timeFuncName early-return for a
// CallExpr whose Fun is a bare Ident.
func TestBareCallOnLocalFunc_OK(_ *testing.T) {
	helper(func() {})
}

// TestSuppressTrailing_OK suppresses a report via a trailing nolint comment.
func TestSuppressTrailing_OK(_ *testing.T) {
	time.Sleep(0) //nolint:synctestcheck // intentional real-clock wait
}

// TestSuppressLeading_OK suppresses a report via a nolint comment on the
// preceding line.
func TestSuppressLeading_OK(_ *testing.T) {
	//nolint:synctestcheck // intentional real-clock wait
	time.Sleep(0)
}

// TestSuppressViaNolintAll_OK confirms the `all` shorthand silences this
// analyzer too.
func TestSuppressViaNolintAll_OK(_ *testing.T) {
	time.Sleep(0) //nolint:all // intentional real-clock wait
}

// TestSuppressInGroup_OK confirms the analyzer name can sit anywhere in a
// comma-separated nolint list.
func TestSuppressInGroup_OK(_ *testing.T) {
	time.Sleep(0) //nolint:gocritic,synctestcheck,unused // intentional real-clock wait
}

// TestUnrelatedNolint_Bad confirms that nolint directives for *other*
// analyzers do not silence this one.
func TestUnrelatedNolint_Bad(_ *testing.T) {
	time.Sleep(0) //nolint:gosec // want `prefer testing/synctest \(synctest.Test\) over real-clock time.Sleep in tests`
}
