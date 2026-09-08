package cas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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

	buf, tlm := newConsoleTelemeter(t, l)

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
	}, collectFallbackAttrs(t, buf), "expected one cas_fallback span attributed to the failed probe")
}

// TestLinkTreeEmitsOneSpanPerTree pins the materialization telemetry
// contract: a tree of many files is reported as a single span carrying what
// the whole tree cost, rather than a span for every file in it.
func TestLinkTreeEmitsOneSpanPerTree(t *testing.T) {
	t.Parallel()

	const files = 200

	l := logger.CreateLogger()

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))

	store := cas.NewStore("/store")
	content := cas.NewContent(store)

	treeData := make([]byte, 0, files*64)

	for i := range files {
		hash := fmt.Sprintf("%040x", i+1)

		require.NoError(t, content.Store(l, v, hash, fmt.Appendf(nil, "content %d\n", i), cas.StoredFilePerms))

		treeData = append(treeData, fmt.Appendf(nil, "100644 blob %s\tmain%d.tf\n", hash, i)...)
	}

	tree, err := git.ParseTree(treeData, "/target")
	require.NoError(t, err)

	buf, tlm := newConsoleTelemeter(t, l)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	require.NoError(t, cas.LinkTree(ctx, l, v, store, store, tree, "/target"))
	require.NoError(t, tlm.Shutdown(ctx))

	var treeSpans []decodedSpan

	for _, span := range decodeSpans(t, buf) {
		assert.NotEqual(t, "cas_link", span.Name, "a single file must not open a span of its own")

		if span.Name == "cas_link_tree" {
			treeSpans = append(treeSpans, span)
		}
	}

	require.Len(t, treeSpans, 1, "a materialized tree must report exactly one span")

	assert.Equal(t, map[string]any{
		"path":         "/target",
		"mode":         cas.LinkModeHardlink.String(),
		"files_linked": float64(files),
		"files_cloned": float64(0),
		"files_copied": float64(0),
		"bytes_copied": float64(0),
		"fallback":     string(cas.LinkFallbackNone),
	}, treeSpans[0].Attrs)
}

// TestLinkTreeReportsDegradedMode pins the fallback attribute for a tree
// the requested mode could not serve throughout: the span names the mode
// that could not be honoured, rather than reporting that the request was
// met.
func TestLinkTreeReportsDegradedMode(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	blobData := []byte("module content\n")

	v := venvtest.New()
	require.NoError(t, v.FS.MkdirAll("/store", 0o755))

	store := cas.NewStore("/store")
	hash := fmt.Sprintf("%040x", 1)

	require.NoError(t, cas.NewContent(store).Store(l, v, hash, blobData, cas.StoredFilePerms))

	tree, err := git.ParseTree(
		fmt.Appendf(nil, "100644 blob %s\tmain.tf\n", hash),
		"/target",
	)
	require.NoError(t, err)

	v = v.WithFS(&vfs.NoSymlinkFS{FS: v.FS})

	buf, tlm := newConsoleTelemeter(t, l)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	require.NoError(t, cas.LinkTree(
		ctx, l, v, store, store, tree, "/target", cas.WithTreeLinkMode(cas.LinkModeHardlink),
	))
	require.NoError(t, tlm.Shutdown(ctx))

	assert.Equal(t, map[string]any{
		"path":         "/target",
		"mode":         cas.LinkModeHardlink.String(),
		"files_linked": float64(0),
		"files_cloned": float64(0),
		"files_copied": float64(1),
		"bytes_copied": float64(len(blobData)),
		"fallback":     string(cas.LinkFallbackHardlinkUnsupported),
	}, linkTreeSpanAttrs(t, buf))
}

// linkTreeSpanAttrs returns the attributes of the single cas_link_tree span
// the console trace exporter wrote into buf.
func linkTreeSpanAttrs(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var attrs map[string]any

	for _, span := range decodeSpans(t, buf) {
		if span.Name != "cas_link_tree" {
			continue
		}

		require.Nil(t, attrs, "expected exactly one cas_link_tree span")

		attrs = span.Attrs
	}

	require.NotNil(t, attrs, "expected a cas_link_tree span")

	return attrs
}

// newConsoleTelemeter returns a telemeter that writes spans as JSON into the
// buffer it also returns, which is how these tests read back what a code path
// reported.
func newConsoleTelemeter(t *testing.T, l log.Logger) (*bytes.Buffer, *telemetry.Telemeter) {
	t.Helper()

	buf := new(bytes.Buffer)

	tlm, err := telemetry.NewTelemeter(
		t.Context(),
		l,
		"terragrunt",
		"v0.0.0-test",
		buf,
		&telemetry.Options{
			TraceExporter: "console",
		},
		false,
	)
	require.NoError(t, err)
	require.NotNil(t, tlm)

	return buf, tlm
}

// decodedSpan is one span as the console trace exporter wrote it.
type decodedSpan struct {
	// Attrs holds the span's attributes, with numbers decoded as float64 the
	// way encoding/json reports any JSON number.
	Attrs map[string]any
	// Name is the span name.
	Name string
}

// decodeSpans reads back every span the console trace exporter wrote into buf.
func decodeSpans(t *testing.T, buf *bytes.Buffer) []decodedSpan {
	t.Helper()

	type exported struct {
		Name       string `json:"Name"`
		Attributes []struct {
			Value struct {
				Value any `json:"Value"`
			} `json:"Value"`
			Key string `json:"Key"`
		} `json:"Attributes"`
	}

	var spans []decodedSpan

	dec := json.NewDecoder(buf)
	for dec.More() {
		var e exported

		require.NoError(t, dec.Decode(&e))

		span := decodedSpan{Name: e.Name, Attrs: map[string]any{}}
		for _, attr := range e.Attributes {
			span.Attrs[attr.Key] = attr.Value.Value
		}

		spans = append(spans, span)
	}

	return spans
}

// collectFallbackAttrs returns the string attributes of the cas_fallback span
// the console trace exporter wrote into buf. Encountering more than one
// cas_fallback span fails the test, so the returned map is unambiguous.
func collectFallbackAttrs(t *testing.T, buf *bytes.Buffer) map[string]string {
	t.Helper()

	var attrs map[string]string

	for _, span := range decodeSpans(t, buf) {
		if span.Name != "cas_fallback" {
			continue
		}

		require.Nil(t, attrs, "expected exactly one cas_fallback span")

		attrs = map[string]string{}

		for key, value := range span.Attrs {
			val, ok := value.(string)
			require.True(t, ok, "cas_fallback attribute %q must be a string", key)

			attrs[key] = val
		}
	}

	return attrs
}
