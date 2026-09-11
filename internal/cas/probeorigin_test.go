package cas_test

import (
	"context"
	"path/filepath"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// probeOriginAttr is the span attribute [cas.CAS.FetchSource] stamps with
// where the probe answer came from.
const probeOriginAttr = "probe_origin"

// TestProbeOriginStampedOnlyByFetchSource pins which span carries
// probe_origin. A fetch reports the origin of the probe it ran, while a
// probe run outside a fetch, the shape stack generation uses to resolve a
// component's ref, leaves its caller's span alone rather than labelling
// that span with a fetch attribute.
func TestProbeOriginStampedOnlyByFetchSource(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	tracer := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)).Tracer("cas_test")

	stub := &lsRemoteStub{hash: stubHash}
	v := stub.venv().WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	c, err := cas.New(v, cas.WithStorePath(filepath.Join(helpers.TmpDirWOSymlinks(t), "store")))
	require.NoError(t, err)

	fetchCtx, fetchSpan := tracer.Start(t.Context(), "cas_fetch_source")
	require.NoError(t, c.FetchSource(fetchCtx, l, v, &cas.CloneOptions{Dir: filepath.Join(t.TempDir(), "dst")},
		cas.SourceRequest{
			Scheme:       "git",
			URL:          "https://example.com/probe-origin.git",
			Resolver:     &cas.GitResolver{Venv: v, Branch: "main"},
			Fetch:        ingestFixture(t, c),
			ProbeCaching: cas.ProbeCachedByResolver,
		}))
	fetchSpan.End()

	stackCtx, stackSpan := tracer.Start(t.Context(), "stack_generate_unit")
	_, err = (&cas.GitResolver{Venv: v, Branch: "main"}).
		Probe(stackCtx, "https://example.com/probe-origin.git")
	require.NoError(t, err)
	stackSpan.End()

	origins := map[string]string{}

	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			if string(attr.Key) == probeOriginAttr {
				origins[span.Name()] = attr.Value.AsString()
			}
		}
	}

	assert.Equal(t, map[string]string{"cas_fetch_source": "ls_remote"}, origins)
}

// ingestFixture returns a fetcher that writes one file into a temporary
// directory and ingests it under the probe's key.
func ingestFixture(t *testing.T, c *cas.CAS) cas.SourceFetcher {
	t.Helper()

	return func(_ context.Context, l log.Logger, v *venv.Venv, suggestedKey string) (string, error) {
		dir := t.TempDir()
		if err := vfs.WriteFile(v.FS, filepath.Join(dir, "main.tf"), []byte("# hello\n"), cas.RegularFilePerms); err != nil {
			return "", err
		}

		return c.IngestDirectory(l, v, dir, suggestedKey)
	}
}
