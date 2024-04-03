package util

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-getter/v2"
)

// FetchFile downloads the file from the given `downloadURL` into the specified `saveToFile` file.
func FetchFile(ctx context.Context, downloadURL, saveToFile string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	resp, err := (&http.Client{
		Timeout: time.Minute * 1,
	}).Do(req)
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

	if written, err := getter.Copy(ctx, out, resp.Body); err != nil {
		return errors.WithStackTrace(err)
	} else if written != resp.ContentLength {
		return errors.Errorf("failed to fetch %s, original size %d but fetched size %d", downloadURL, resp.ContentLength, written)
	}

	if err := out.Sync(); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
