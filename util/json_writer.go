package util

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	MsgKey = "msg"
)

type JsonWriter struct {
	writer     io.Writer
	jsonFields map[string]interface{}
}

// NewJsonWriter creates a new JsonWriter that writes to the provided io.Writer
// and can accept initial JSON fields.
func NewJsonWriter(w io.Writer, fields map[string]interface{}) *JsonWriter {
	return &JsonWriter{
		writer:     w,
		jsonFields: fields,
	}
}

// Write accepts a byte slice, wraps it within a JSON object with the message under "msg"
// and merges additional JSON fields from the jsonFields map.
func (j *JsonWriter) Write(p []byte) (int, error) {
	data := make(map[string]interface{})
	for k, v := range j.jsonFields {
		data[k] = v
	}

	data[MsgKey] = string(p)

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("error marshaling JSON: %w", err)
	}

	n, err := j.writer.Write(jsonBytes)
	if err != nil {
		return n, fmt.Errorf("error writing JSON: %w", err)
	}
	return len(p), nil
}
