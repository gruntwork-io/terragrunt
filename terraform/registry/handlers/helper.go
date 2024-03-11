package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

func ModifyJSONBody(resp *http.Response, body any, fn func() error) error {
	if resp.StatusCode != http.StatusOK {
		return nil
	}

	buffer := new(bytes.Buffer)

	if _, err := buffer.ReadFrom(resp.Body); err != nil {
		return err
	}

	decoder := json.NewDecoder(buffer)
	if err := decoder.Decode(body); err != nil {
		return err
	}

	if err := fn(); err != nil {
		return err
	}

	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(body); err != nil {
		return err
	}

	resp.Body = io.NopCloser(buffer)
	resp.ContentLength = int64(buffer.Len())

	return nil
}
