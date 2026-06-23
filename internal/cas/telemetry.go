package cas

import (
	"context"
	"maps"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// FallbackReason classifies why a CAS code path degraded to a slower
// alternative. It travels as the reason attribute on the cas_fallback
// telemetry event emitted by [RecordFallback].
type FallbackReason string

const (
	// FallbackReasonInitError reports that the CAS store or its execution
	// environment could not be initialized, so the source was downloaded
	// through the standard getter without CAS.
	FallbackReasonInitError FallbackReason = "init_error"

	// FallbackReasonGetterError reports that the CAS-backed download
	// failed and the source was re-downloaded through the standard
	// getter.
	FallbackReasonGetterError FallbackReason = "getter_error"

	// FallbackReasonGitStoreUnavailable reports that the central
	// [GitStore] could not serve a ref or commit (e.g. locked or
	// corrupted), so the fetch fell back to a temporary clone.
	FallbackReasonGitStoreUnavailable FallbackReason = "git_store_unavailable"

	// FallbackReasonProbeFailure reports that a [SourceResolver.Probe]
	// failed, so [CAS.FetchSource] could not short-circuit on a cached
	// tree and re-downloaded the source.
	FallbackReasonProbeFailure FallbackReason = "probe_failure"

	// FallbackReasonStackGenerationError reports that CAS-backed stack
	// generation failed for a component, falling back to the standard
	// copy or getter path.
	FallbackReasonStackGenerationError FallbackReason = "stack_generation_error"
)

// RecordFallback emits a cas_fallback telemetry event so operators can
// measure how often CAS degrades and why. reason distinguishes the
// fallback causes; attrs carry site-specific context (e.g. the source
// URL) and may be nil.
//
// The telemeter travels on ctx, matching [CAS.FetchSource]. A failure
// to record the event is logged at debug level and never affects the
// fallback itself.
func RecordFallback(ctx context.Context, l log.Logger, reason FallbackReason, attrs map[string]any) {
	all := map[string]any{"reason": string(reason)}

	maps.Copy(all, attrs)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_fallback", all, func(context.Context) error {
		return nil
	})
	if err != nil {
		l.Debugf("cas: failed to record fallback telemetry: %v", err)
	}
}
