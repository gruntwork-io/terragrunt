package getter_test

import (
	"errors"
	"iter"
	"strings"
	"testing"

	upstream "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
)

// namesFrom returns a sequence over names for [getter.ScanMode], plus a
// counter of how many the scan actually pulled.
func namesFrom(names []string) (iter.Seq2[string, error], *int) {
	var yielded int

	return func(yield func(string, error) bool) {
		for _, name := range names {
			yielded++

			if !yield(name, nil) {
				return
			}
		}
	}, &yielded
}

// dirOnPrefix resolves directory mode for anything below key/ and file mode on
// an exact match, mirroring what the s3 getter asks of the scan.
func dirOnPrefix(key string) func(string) (upstream.Mode, bool) {
	return func(name string) (upstream.Mode, bool) {
		if name == key {
			return upstream.ModeFile, true
		}

		if strings.HasPrefix(name, key+"/") {
			return upstream.ModeDir, true
		}

		return 0, false
	}
}

// TestScanModeStopsAtLimit pins that the scan gives up rather than guessing
// once it has inspected limit entries, and that it stops pulling at exactly
// that many.
func TestScanModeStopsAtLimit(t *testing.T) {
	t.Parallel()

	next, pulled := namesFrom([]string{"mod1", "mod2", "mod3", "mod4"})

	_, err := getter.ScanMode(2, next, dirOnPrefix("mod"))
	require.ErrorIs(t, err, getter.ErrModeScanLimit)
	assert.Equal(t, 2, *pulled, "the scan must stop pulling at the limit")
}

// TestScanModeResolvesWithinLimit pins that a decision reached before the cap
// wins, so a low limit does not turn a resolvable prefix into an error.
func TestScanModeResolvesWithinLimit(t *testing.T) {
	t.Parallel()

	next, _ := namesFrom([]string{"mod/a.tf", "mod/b.tf"})

	mode, err := getter.ScanMode(2, next, dirOnPrefix("mod"))
	require.NoError(t, err)
	assert.Equal(t, upstream.ModeDir, mode)
}

// TestScanModeExhaustedIsFile pins that a listing that runs out without
// resolving means the prefix names a file, matching what the getters relied on
// before the cap existed.
func TestScanModeExhaustedIsFile(t *testing.T) {
	t.Parallel()

	next, _ := namesFrom([]string{"modular", "moderate"})

	mode, err := getter.ScanMode(getter.DefaultModeScanLimit, next, dirOnPrefix("mod"))
	require.NoError(t, err)
	assert.Equal(t, upstream.ModeFile, mode)
}

// TestScanModePropagatesListError pins that a listing failure surfaces as
// itself rather than as a limit error.
func TestScanModePropagatesListError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("list failed")
	failing := func(yield func(string, error) bool) { yield("", sentinel) }

	_, err := getter.ScanMode(getter.DefaultModeScanLimit, failing, dirOnPrefix("mod"))
	require.ErrorIs(t, err, sentinel)
	require.NotErrorIs(t, err, getter.ErrModeScanLimit)
}
