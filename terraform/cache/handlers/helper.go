package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func DecodeJSONBody(resp *http.Response, value any) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	buffer := new(bytes.Buffer)

	if _, err := buffer.ReadFrom(resp.Body); err != nil {
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

	buffer := new(bytes.Buffer)

	if _, err := buffer.ReadFrom(resp.Body); err != nil {
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

	return nil
}
