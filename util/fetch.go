package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-getter/v2"
)

func Fetch(ctx context.Context, url string, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned from %s", resp.Status, url)
	}

	if written, err := getter.Copy(ctx, dst, resp.Body); err != nil {
		return errors.WithStackTrace(err)
	} else if written != resp.ContentLength {
		return errors.Errorf("incorrect response size: expected %d bytes, but got %d bytes", resp.ContentLength, written)
	}

	return nil
}

// Fetch downloads the file from the given `url` into the specified `dst` file.
func FetchToFile(ctx context.Context, url, dst string) error {
	file, err := os.Create(dst)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer file.Close() //nolint:errcheck

	if err := Fetch(ctx, url, file); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}
