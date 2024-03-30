package util

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// FetchFile downloads the file from the given `downloadURL` into the specified `saveToFile` file.
func FetchFile(ctx context.Context, downloadURL, saveToFile string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to fetch %s, server response %q", downloadURL, http.StatusText(resp.StatusCode))
	}

	out, err := os.Create(saveToFile)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer out.Close() //nolint:errcheck

	if written, err := io.Copy(out, resp.Body); err != nil {
		return errors.WithStackTrace(err)
	} else if written != resp.ContentLength {
		errors.Errorf("file fetched incompletely, remote size %d, but fetched size %d", resp.ContentLength, written)
	}
	return nil
}

func FetchFileWithRetry(ctx context.Context, downloadURL, saveToFile string, maxRetries int, retryDelay time.Duration) error {
	var retry int

	for {
		err := FetchFile(ctx, downloadURL, saveToFile)
		if err == nil || retry >= maxRetries {
			return err
		}

		retry++
		log.Tracef("%v, next (%d of %d) attempt in %v", err, retry, maxRetries, retryDelay)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
			// try again
		}
	}
}
