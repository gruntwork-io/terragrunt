package tf

import (
	"bytes"
	"encoding/json"
)

// ExtractFirstJSONObject returns the first complete JSON object found in data, ignoring any
// non-JSON content that precedes or follows it. This is needed because `tofu/terraform output -json`
// can intermix log lines, ANSI escape codes, or deprecation warnings with the JSON output, depending
// on the version and backend in use.
//
// If data contains no `{`, the original bytes are returned so downstream JSON parsing surfaces the
// usual "unexpected end of JSON input" error rather than a cryptic message from this helper.
func ExtractFirstJSONObject(data []byte) ([]byte, error) {
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return data, nil
	}

	dec := json.NewDecoder(bytes.NewReader(data[start:]))

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	return raw, nil
}
