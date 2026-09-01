// Package helpers provides utility functions for working with HTTP requests and responses.
package helpers

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

// MaxJSONResponseBytes bounds what [ResponseBuffer] reads into memory.
// Everything it reads is registry metadata, orders of magnitude smaller than
// this.
const MaxJSONResponseBytes = 1 << 20

// ResponseTooLargeError is returned when a response outgrows the limit it was
// read under.
type ResponseTooLargeError struct {
	Limit int64
}

func (err ResponseTooLargeError) Error() string {
	return fmt.Sprintf("response exceeds the limit of %d bytes", err.Limit)
}

// Fetch dispatches req through c and copies the (possibly gzip-decoded)
// response body into dst, up to limit bytes. The limit applies after decoding,
// because the server decides how far a compressed response expands and the
// declared content length says nothing about it.
func Fetch(ctx context.Context, c vhttp.Client, req *http.Request, dst io.Writer, limit int64) error {
	req.Header.Add("Accept-Encoding", "gzip")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned from %s", resp.Status, req.URL)
	}

	reader, err := ResponseReader(resp)
	if err != nil {
		return err
	}

	// Read one byte past the limit to tell a response that outgrew it from one
	// that ends exactly on it.
	written, err := util.Copy(ctx, dst, io.LimitReader(reader, limit+1))
	if err != nil {
		return err
	}

	if written > limit {
		return fmt.Errorf("fetching %s: %w", req.URL, ResponseTooLargeError{Limit: limit})
	}

	if resp.ContentLength != -1 && written != resp.ContentLength {
		return fmt.Errorf(
			"incorrect response size: expected %d bytes, but got %d bytes",
			resp.ContentLength,
			written,
		)
	}

	return nil
}

// FetchToFile downloads req's response through c into the file at dst, up to
// limit bytes.
func FetchToFile(
	ctx context.Context,
	c vhttp.Client,
	fsys vfs.FS,
	req *http.Request,
	dst string,
	limit int64,
) error {
	file, err := fsys.Create(dst)
	if err != nil {
		return err
	}
	defer file.Close() //nolint:errcheck

	if err := Fetch(ctx, c, req, file, limit); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
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

// ResponseBuffer reads resp's (possibly gzip-decoded) body into a buffer,
// refusing a body that outgrows [MaxJSONResponseBytes].
func ResponseBuffer(resp *http.Response) (*bytes.Buffer, error) {
	reader, err := ResponseReader(resp)
	if err != nil {
		return nil, err
	}
	defer reader.Close() //nolint:errcheck

	buffer := new(bytes.Buffer)

	read, err := buffer.ReadFrom(io.LimitReader(reader, MaxJSONResponseBytes+1))
	if err != nil {
		return nil, err
	}

	if read > MaxJSONResponseBytes {
		return nil, ResponseTooLargeError{Limit: MaxJSONResponseBytes}
	}

	return buffer, nil
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
