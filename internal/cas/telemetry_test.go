package cas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetchSource_ProbeFailureRecordsFallback pins the cas_fallback
// telemetry contract on the probe-failure site: a failing probe must
// emit a span named cas_fallback carrying reason=probe_failure while
// the fetch itself still succeeds through the download path.
func TestFetchSource_ProbeFailureRecordsFallback(t *testing.T) {
	t.Parallel()

	c, v := newCAS(t)
	l := logger.CreateLogger()

	var buf bytes.Buffer

	tlm, err := telemetry.NewTelemeter(
		t.Context(),
		l,
		"terragrunt",
		"v0.0.0-test",
		&buf,
		&telemetry.Options{
			TraceExporter: "console",
		},
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, tlm)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	resolver := &fakeResolver{
		scheme: "http",
		err:    errors.New("probe exploded"),
	}

	var fetchCalls atomic.Int32

	dst := filepath.Join(t.TempDir(), "dst")
	require.NoError(t, c.FetchSource(ctx, l, v, &cas.CloneOptions{Dir: dst}, cas.SourceRequest{
		Scheme:   "http",
		URL:      "https://example.com/mod.tgz",
		Resolver: resolver,
		Fetch:    fakeFetcher(c, map[string]string{"main.tf": "ok"}, &fetchCalls),
	}))
	require.NoError(t, tlm.Shutdown(ctx))

	assert.Equal(t, int32(1), fetchCalls.Load(), "probe failure must still download the source")
	assert.FileExists(t, filepath.Join(dst, "main.tf"))

	assert.Equal(t, map[string]string{
		"reason": string(cas.FallbackReasonProbeFailure),
		"scheme": "http",
		"url":    "https://example.com/mod.tgz",
	}, collectFallbackAttrs(t, &buf), "expected one cas_fallback span attributed to the failed probe")
}

// collectFallbackAttrs decodes the console trace exporter output in buf
// and returns the string attributes of the cas_fallback spans it finds.
// Encountering more than one cas_fallback span fails the test, so the
// returned map is unambiguous.
func collectFallbackAttrs(t *testing.T, buf *bytes.Buffer) map[string]string {
	t.Helper()

	// Value is any rather than string because the buffer interleaves
	// spans from the whole fetch, and cas_link spans carry bool and
	// int attributes.
	type span struct {
		Name       string `json:"Name"`
		Attributes []struct {
			Value struct {
				Value any `json:"Value"`
			} `json:"Value"`
			Key string `json:"Key"`
		} `json:"Attributes"`
	}

	var attrs map[string]string

	dec := json.NewDecoder(buf)
	for dec.More() {
		var s span
		require.NoError(t, dec.Decode(&s))

		if s.Name != "cas_fallback" {
			continue
		}

		require.Nil(t, attrs, "expected exactly one cas_fallback span")

		attrs = map[string]string{}

		for _, attr := range s.Attributes {
			val, ok := attr.Value.Value.(string)
			require.True(t, ok, "cas_fallback attribute %q must be a string", attr.Key)

			attrs[attr.Key] = val
		}
	}

	return attrs
}
