package diagnostic

import "github.com/hashicorp/hcl/v2"

func ExtraInfo[T any](diag *hcl.Diagnostic) T {
	extra := diag.Extra
	if ret, ok := extra.(T); ok {
		return ret
	}

	// If "extra" doesn't implement T directly then we'll delegate to our ExtraInfoNext helper to try iteratively unwrapping it.
	return ExtraInfoNext[T](extra)
}

// ExtraInfoNext takes a value previously returned by ExtraInfo and attempts to find an implementation of interface T wrapped inside of it. The return value meaning is the same as for ExtraInfo.
func ExtraInfoNext[T any](previous interface{}) T {
	// As long as T is an interface type as documented, zero will always be a nil interface value for us to return in the non-matching case.
	var zero T

	unwrapper, ok := previous.(DiagnosticExtraUnwrapper)
	// If the given value isn't unwrappable then it can't possibly have any other info nested inside of it.
	if !ok {
		return zero
	}

	extra := unwrapper.UnwrapDiagnosticExtra()

	// Keep unwrapping until we either find the interface to look for or we run out of layers of unwrapper.
	for {
		if ret, ok := extra.(T); ok {
			return ret
		}

		if unwrapper, ok := extra.(DiagnosticExtraUnwrapper); ok {
			extra = unwrapper.UnwrapDiagnosticExtra()
		} else {
			return zero
		}
	}
}

// DiagnosticExtraUnwrapper is an interface implemented by values in the Extra field of Diagnostic when they are wrapping another "Extra" value that was generated downstream.
type DiagnosticExtraUnwrapper interface {
	UnwrapDiagnosticExtra() interface{}
}

// DiagnosticExtraBecauseUnknown is an interface implemented by values in the Extra field of Diagnostic when the diagnostic is potentially caused by the presence of unknown values in an expression evaluation.
type DiagnosticExtraBecauseUnknown interface {
	DiagnosticCausedByUnknown() bool
}

// DiagnosticCausedByUnknown returns true if the given diagnostic has an indication that it was caused by the presence of unknown values during an expression evaluation.
func DiagnosticCausedByUnknown(diag *hcl.Diagnostic) bool {
	maybe := ExtraInfo[DiagnosticExtraBecauseUnknown](diag)
	if maybe == nil {
		return false
	}

	return maybe.DiagnosticCausedByUnknown()
}

// DiagnosticExtraBecauseSensitive is an interface implemented by values in the Extra field of Diagnostic when the diagnostic is potentially caused by the presence of sensitive values in an expression evaluation.
type DiagnosticExtraBecauseSensitive interface {
	DiagnosticCausedBySensitive() bool
}

// DiagnosticCausedBySensitive returns true if the given diagnostic has an/ indication that it was caused by the presence of sensitive values during an expression evaluation.
func DiagnosticCausedBySensitive(diag *hcl.Diagnostic) bool {
	maybe := ExtraInfo[DiagnosticExtraBecauseSensitive](diag)
	if maybe == nil {
		return false
	}

	return maybe.DiagnosticCausedBySensitive()
}
