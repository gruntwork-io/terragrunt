package handlers

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

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
