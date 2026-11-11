package gcs_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/storage"
	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

const progressBucket = "progress-state-bucket"

// TestGCSBucketWaitProgress checks that the GCS bucket wait is drawn and logs its done line only with a reporter.
func TestGCSBucketWaitProgress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		wantLogLines int
		reporter     bool
	}{
		{name: "reporter on the context", reporter: true, wantLogLines: 1},
		{name: "plain context"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stub := &gcsStub{}
			term := new(bytes.Buffer)

			logs := new(bytes.Buffer)
			l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))

			ctx := t.Context()
			if tc.reporter {
				ctx = spinner.ContextWithReporter(ctx, drawingReporter(term))
			}

			require.NoError(t, newStubbedClient(t, stub).CreateGCSBucketWithVersioning(ctx, l, progressBucket))
			require.Positive(t, stub.polls.Load(), "the wait must have polled the endpoint")

			assert.Equal(t, tc.reporter, term.Len() > 0)
			assert.Equal(t, tc.wantLogLines, strings.Count(logs.String(), "\n"), logs)
		})
	}
}

type gcsStub struct {
	polls atomic.Int64
}

func (stub *gcsStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body := `{"name":"` + progressBucket + `"}`

	if r.Method == http.MethodGet {
		stub.polls.Add(1)
	}

	if strings.HasSuffix(r.URL.Path, "/o") {
		body = `{"kind":"storage#objects"}`
	}

	if _, err := w.Write([]byte(body)); err != nil {
		panic(err)
	}
}

func newStubbedClient(t *testing.T, stub *gcsStub) *gcsbackend.Client {
	t.Helper()

	srv := httptest.NewServer(stub)
	t.Cleanup(srv.Close)

	storageClient, err := storage.NewClient(
		t.Context(),
		option.WithEndpoint(srv.URL+"/storage/v1/"),
		option.WithoutAuthentication(),
		option.WithHTTPClient(srv.Client()),
	)
	require.NoError(t, err)

	return &gcsbackend.Client{
		ExtendedRemoteStateConfigGCS: &gcsbackend.ExtendedRemoteStateConfigGCS{
			Project:  "progress-project",
			Location: "us",
		},
		Client: storageClient,
	}
}

// drawingReporter returns a reporter that draws its progress line on term from the start.
func drawingReporter(term io.Writer) *spinner.Reporter {
	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{"TERM": "xterm"}).WithGOOS("linux")
	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return 120 }

	return spinner.New(v, spinner.Options{LogsForHumans: true})
}
