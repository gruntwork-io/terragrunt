package router_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseWriterRecordsFirstStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := router.NewResponseWriter(rec)

	assert.Equal(t, 0, w.Status(), "status should be unset before anything is sent")

	w.WriteHeader(http.StatusAccepted)
	w.WriteHeader(http.StatusInternalServerError)

	assert.Equal(t, http.StatusAccepted, w.Status(), "a later status should be dropped")
	assert.Equal(t, http.StatusAccepted, rec.Code)
}

func TestResponseWriterWriteSendsOK(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := router.NewResponseWriter(rec)

	n, err := w.Write([]byte("body"))
	require.NoError(t, err)

	assert.Equal(t, 4, n)
	assert.Equal(t, http.StatusOK, w.Status(), "writing a body should send a 200")
	assert.Equal(t, "body", rec.Body.String())
}

func TestResponseWriterUnwrapReachesUnderlyingWriter(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	w := router.NewResponseWriter(rec)

	require.NoError(t, http.NewResponseController(w).Flush(), "flush should reach the recorder through Unwrap")
	assert.True(t, rec.Flushed)
}

func TestWriteError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err        error
		name       string
		wantBody   string
		sentStatus int
		wantStatus int
	}{
		{
			name:       "http error sets status and message",
			err:        &router.HTTPError{Code: http.StatusTeapot, Message: "short and stout"},
			wantStatus: http.StatusTeapot,
			wantBody:   "short and stout\n",
		},
		{
			name:       "wrapped http error is found",
			err:        errors.Join(errors.New("context"), router.NewHTTPError(http.StatusNotFound)),
			wantStatus: http.StatusNotFound,
			wantBody:   http.StatusText(http.StatusNotFound) + "\n",
		},
		{
			name:       "other error hides its text behind a 500",
			err:        errors.New("open /secret/path: permission denied"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   http.StatusText(http.StatusInternalServerError) + "\n",
		},
		{
			name:       "response already under way is left alone",
			err:        router.NewHTTPError(http.StatusNotFound),
			sentStatus: http.StatusAccepted,
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			w := router.NewResponseWriter(rec)

			if tc.sentStatus != 0 {
				w.WriteHeader(tc.sentStatus)
			}

			router.WriteError(w, tc.err)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}

func TestHTTPErrorMessage(t *testing.T) {
	t.Parallel()

	err := router.NewHTTPError(http.StatusNotFound)

	assert.Equal(t, http.StatusNotFound, err.Code)
	assert.Equal(t, http.StatusText(http.StatusNotFound), err.Message)
	assert.Equal(t, "code=404, message=Not Found", err.Error())
}

func TestJSON(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()

	require.NoError(t, router.JSON(rec, http.StatusCreated, map[string]string{"id": "aws"}))

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "application/json; charset=UTF-8", rec.Header().Get("Content-Type"))

	var got map[string]string

	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, map[string]string{"id": "aws"}, got)
}

func TestJSONUnencodableValue(t *testing.T) {
	t.Parallel()

	var unsupported *json.UnsupportedTypeError

	require.ErrorAs(t, router.JSON(httptest.NewRecorder(), http.StatusOK, make(chan int)), &unsupported)
}
