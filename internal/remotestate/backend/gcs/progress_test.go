package gcs_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/storage"
	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
)

const progressBucket = "progress-state-bucket"

func TestGCSBucketWaitIsReported(t *testing.T) {
	t.Parallel()

	working := "Waiting for GCS bucket " + progressBucket + " to be created..."
	done := "GCS bucket " + progressBucket + " is ready"

	t.Run("experiment on", func(t *testing.T) {
		t.Parallel()

		stub := &gcsStub{}

		logs := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))
		ctx := spinner.ContextWithReporter(t.Context(), spinner.New(spinner.Options{}))

		require.NoError(t, newStubbedClient(t, stub).CreateGCSBucketWithVersioning(ctx, l, progressBucket))
		require.Positive(t, stub.polls.Load(), "the wait must have polled the endpoint")

		assert.Contains(t, logs.String(), working)
		assert.Contains(t, logs.String(), done)
	})

	t.Run("experiment off", func(t *testing.T) {
		t.Parallel()

		stub := &gcsStub{}

		logs := new(bytes.Buffer)
		l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))

		require.NoError(t, newStubbedClient(t, stub).CreateGCSBucketWithVersioning(t.Context(), l, progressBucket))
		require.Positive(t, stub.polls.Load(), "the wait must have polled the endpoint")

		assert.Empty(t, logs.String(), "nothing is logged at INFO without the reporter")
	})
}

// gcsStub answers the bucket create, bucket attrs and object list calls.
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
