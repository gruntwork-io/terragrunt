package util

import (
	"bytes"
	"encoding/json"
	"github.com/go-errors/errors"
	"io"
	"reflect"
	"testing"
)

type MockWriter struct {
	failWrite bool
}

func (m *MockWriter) Write(p []byte) (n int, err error) {
	if m.failWrite {
		return 0, errors.New("mock writer forced error")
	}
	return len(p), nil
}

func TestJsonWriter_Write(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		input       string
		writer      io.Writer
		jsonFields  map[string]interface{}
		wantErr     bool
		checkFields bool
	}{
		{
			name:        "Basic functionality",
			input:       "Hello, world!",
			writer:      new(bytes.Buffer),
			jsonFields:  map[string]interface{}{"level": "INFO"},
			wantErr:     false,
			checkFields: true,
		},
		{
			name:        "Special characters in message",
			input:       "Hello, world! 😀",
			writer:      new(bytes.Buffer),
			jsonFields:  map[string]interface{}{"level": "DEBUG"},
			wantErr:     false,
			checkFields: true,
		},
		{
			name:        "Empty message",
			input:       "",
			writer:      new(bytes.Buffer),
			jsonFields:  map[string]interface{}{"level": "WARN"},
			wantErr:     false,
			checkFields: true,
		},
		{
			name:        "No additional fields",
			input:       "Hello, world!",
			writer:      new(bytes.Buffer),
			jsonFields:  map[string]interface{}{},
			wantErr:     false,
			checkFields: true,
		},
		{
			name:        "Writer error handling",
			input:       "Test message",
			writer:      &MockWriter{failWrite: true},
			jsonFields:  map[string]interface{}{"level": "CRITICAL"},
			wantErr:     true,
			checkFields: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf, ok := tc.writer.(*bytes.Buffer)
			writer := NewJsonWriter(tc.writer, tc.jsonFields)
			_, err := writer.Write([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tc.wantErr)
			}

			if ok && tc.checkFields && !tc.wantErr {
				var gotData map[string]interface{}
				if err := json.Unmarshal(buf.Bytes(), &gotData); err != nil {
					t.Errorf("Error unmarshaling result: %v", err)
				}

				if gotMsg, exists := gotData["msg"].(string); !exists || gotMsg != tc.input {
					t.Errorf("Write() got message = %v, want %v", gotMsg, tc.input)
				}

				for k, v := range tc.jsonFields {
					if gotVal, exists := gotData[k]; !exists || !reflect.DeepEqual(gotVal, v) {
						t.Errorf("Write() got %v = %v, want %v", k, gotVal, v)
					}
				}
			}
		})
	}
}
