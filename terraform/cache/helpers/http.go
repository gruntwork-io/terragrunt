package helpers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/go-getter/v2"
)

func Fetch(ctx context.Context, req *http.Request, dst io.Writer) error {
	req.Header.Add("Accept-Encoding", "gzip")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned from %s", resp.Status, req.URL)
	}

	reader, err := ResponseReader(resp)
	if err != nil {
		return err
	}

	if written, err := getter.Copy(ctx, dst, reader); err != nil {
		return errors.WithStackTrace(err)
	} else if resp.ContentLength != -1 && written != resp.ContentLength {
		return errors.Errorf("incorrect response size: expected %d bytes, but got %d bytes", resp.ContentLength, written)
	}

	return nil
}

// FetchToFile downloads the file from the given `url` into the specified `dst` file.
func FetchToFile(ctx context.Context, req *http.Request, dst string) error {
	file, err := os.Create(dst)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer file.Close() //nolint:errcheck

	if err := Fetch(ctx, req, file); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func ResponseReader(resp *http.Response) (io.ReadCloser, error) {
	// Check that the server actually sent compressed data
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}

		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true

		return reader, nil
	default:
		return resp.Body, nil
	}
}

func ResponseBuffer(resp *http.Response) (*bytes.Buffer, error) {
	reader, err := ResponseReader(resp)
	if err != nil {
		return nil, err
	}
	defer reader.Close() //nolint:errcheck

	buffer := new(bytes.Buffer)

	if _, err := buffer.ReadFrom(reader); err != nil {
		return nil, err
	}

	return buffer, nil
}

func DecodeJSONBody(resp *http.Response, value any) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	buffer, err := ResponseBuffer(resp)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(buffer)
	if err := decoder.Decode(value); err != nil {
		return err
	}

	resp.Body = io.NopCloser(buffer)
	return nil
}

func ModifyJSONBody(resp *http.Response, value any, fn func() error) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	buffer, err := ResponseBuffer(resp)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(buffer)
	if err := decoder.Decode(value); err != nil {
		return err
	}

	if fn == nil {
		return nil
	}

	if err := fn(); err != nil {
		return err
	}

	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(value); err != nil {
		return err
	}

	resp.Body = io.NopCloser(buffer)
	resp.ContentLength = int64(buffer.Len())
	resp.Header.Set("Content-Length", strconv.Itoa(buffer.Len()))

	return nil
}
