package tui

import (
	"fmt"
)

// SourceAccessHint closes each full rendering of a source-load failure: the
// welcome error screen, the post-exit notice, and the error a non-interactive
// run returns. A source that fails is nearly always one the user's git cannot
// reach, and neither the clone error nor the summary above it says what to do
// about that.
const SourceAccessHint = "Check that git can clone each repository listed above.\n" +
	"A private repository needs a credential git can find on its own; the\n" +
	"catalog cannot answer a prompt for one."

// SourceFailure records a single catalog source that failed to load during
// multi-repo discovery, together with the cause.
type SourceFailure struct {
	// Err is the load failure for this source.
	Err error
	// URL is the repository URL of the failed source.
	URL string
}

// SourceLoadError aggregates per-source load failures from a discovery run.
// Attempted counts the distinct sources discovery tried to load, so callers
// can distinguish "every source failed" from a partial failure where the
// catalog still has usable components.
type SourceLoadError struct {
	Failures  []SourceFailure
	Attempted int
}

// AllFailed reports whether every attempted source failed to load.
func (e *SourceLoadError) AllFailed() bool {
	return e.Attempted > 0 && len(e.Failures) == e.Attempted
}

// Error returns a one-line summary. Per-source details live in Failures and
// are rendered by the TUI rather than packed into the error string.
func (e *SourceLoadError) Error() string {
	if e.AllFailed() {
		return fmt.Sprintf("failed to load all %d catalog %s",
			e.Attempted, pluralize("source", "sources", e.Attempted))
	}

	return fmt.Sprintf("failed to load %d of %d catalog sources", len(e.Failures), e.Attempted)
}

// Unwrap exposes the underlying per-source errors to [errors.Is] and
// [errors.As].
func (e *SourceLoadError) Unwrap() []error {
	errs := make([]error, len(e.Failures))
	for i, f := range e.Failures {
		errs[i] = f.Err
	}

	return errs
}
