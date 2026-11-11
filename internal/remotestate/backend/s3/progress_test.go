package s3_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/gruntwork-io/terragrunt/internal/venv"

	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/internal/spinner"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	progressBucket = "progress-state-bucket"
	progressTable  = "progress-lock-table"
)

// TestRemoteStateWaitProgress checks that the S3 bucket and DynamoDB table waits are drawn only with a reporter.
func TestRemoteStateWaitProgress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		run          func(ctx context.Context, l log.Logger, client *s3backend.Client) error
		name         string
		waits        int
		missingHeads int64
		created      bool
	}{
		{
			name: "s3 bucket exists",
			run: func(ctx context.Context, l log.Logger, client *s3backend.Client) error {
				return client.WaitUntilS3BucketExists(ctx, l, progressBucket)
			},
			waits: 1,
		},
		{
			name:         "s3 bucket appears after a retry",
			missingHeads: 1,
			run: func(ctx context.Context, l log.Logger, client *s3backend.Client) error {
				return client.WaitUntilS3BucketExists(ctx, l, progressBucket)
			},
			waits: 1,
		},
		{
			name: "dynamodb table active",
			run: func(ctx context.Context, l log.Logger, client *s3backend.Client) error {
				return client.CreateLockTableIfNecessary(ctx, l, progressTable, nil)
			},
			waits: 1,
		},
		{
			name:    "dynamodb table encryption",
			created: true,
			run: func(ctx context.Context, l log.Logger, client *s3backend.Client) error {
				return client.UpdateLockTableSetSSEncryptionOnIfNecessary(ctx, l, progressTable)
			},
			waits: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			t.Run("reporter on the context", func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					stub := new(awsStub)
					stub.created.Store(tc.created)
					stub.headMisses.Store(tc.missingHeads)

					term := new(bytes.Buffer)
					logs := new(bytes.Buffer)
					l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))
					ctx := spinner.ContextWithReporter(t.Context(), drawingReporter(term))

					require.NoError(t, tc.run(ctx, l, newStubbedClient(t, stub)))
					require.Greater(t, stub.requests.Load(), tc.missingHeads, "the wait must have polled until ready")

					assert.NotEmpty(t, term.String())
					assert.Equal(t, tc.waits, strings.Count(logs.String(), "\n"), logs)
				})
			})

			t.Run("plain context", func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					stub := new(awsStub)
					stub.created.Store(tc.created)
					stub.headMisses.Store(tc.missingHeads)

					logs := new(bytes.Buffer)
					l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(util.NewSyncWriter(logs)))

					require.NoError(t, tc.run(t.Context(), l, newStubbedClient(t, stub)))
					require.Greater(t, stub.requests.Load(), tc.missingHeads, "the wait must have polled until ready")

					assert.Empty(t, logs.String())
				})
			})
		})
	}
}

type awsStub struct {
	requests   atomic.Int64
	headMisses atomic.Int64
	created    atomic.Bool
	encrypted  atomic.Bool
}

func (stub *awsStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stub.requests.Add(1)

	target := r.Header.Get("X-Amz-Target")

	if r.Method == http.MethodHead {
		if stub.headMisses.Add(-1) >= 0 {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.WriteHeader(http.StatusOK)

		return
	}

	w.Header().Set("Content-Type", "application/x-amz-json-1.0")

	if strings.HasSuffix(target, ".CreateTable") {
		stub.created.Store(true)
		writeBody(w, http.StatusOK, `{"TableDescription":{"TableStatus":"CREATING"}}`)

		return
	}

	if strings.HasSuffix(target, ".UpdateTable") {
		stub.encrypted.Store(true)
		writeBody(w, http.StatusOK, `{}`)

		return
	}

	if !strings.HasSuffix(target, ".DescribeTable") {
		writeBody(w, http.StatusInternalServerError, `{"__type":"UnexpectedCall","message":"`+target+`"}`)

		return
	}

	if !stub.created.Load() {
		writeBody(
			w,
			http.StatusBadRequest,
			`{"__type":"com.amazonaws.dynamodb.v20120810#ResourceNotFoundException","message":"not found"}`,
		)

		return
	}

	if stub.encrypted.Load() {
		writeBody(w, http.StatusOK, `{"Table":{"TableStatus":"ACTIVE","SSEDescription":{"Status":"ENABLED"}}}`)

		return
	}

	writeBody(w, http.StatusOK, `{"Table":{"TableStatus":"ACTIVE"}}`)
}

func writeBody(w http.ResponseWriter, status int, body string) {
	w.WriteHeader(status)

	if _, err := w.Write([]byte(body)); err != nil {
		panic(err)
	}
}

func newStubbedClient(t *testing.T, stub *awsStub) *s3backend.Client {
	t.Helper()

	return newMemS3Client(t, func(_ context.Context, req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		stub.ServeHTTP(recorder, req)

		return recorder.Result(), nil
	})
}

// drawingReporter returns a reporter that draws its progress line on term from the start.
func drawingReporter(term io.Writer) *spinner.Reporter {
	v := venv.MemVenv().WithErrWriter(term).WithEnv(map[string]string{"TERM": "xterm"}).WithGOOS("linux")
	v.Terminal.StderrIsTTY = func() bool { return true }
	v.Terminal.StderrHandlesEscapes = func() bool { return true }
	v.Terminal.ErrWidth = func() int { return 120 }

	return spinner.New(v, spinner.Options{LogsForHumans: true})
}
