package view

import (
	"encoding/json"

	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
)

// JSONRender formats output as one line of JSON.
type JSONRender struct{}

// NewJSONRender returns a [JSONRender].
func NewJSONRender() Render {
	return &JSONRender{}
}

// Diagnostics formats diags as a JSON array.
func (render *JSONRender) Diagnostics(diags diagnostic.Diagnostics) (string, error) {
	return render.toJSON(diags)
}

// ShowConfigPath formats filenames as a JSON array of strings.
func (render *JSONRender) ShowConfigPath(filenames []string) (string, error) {
	return render.toJSON(filenames)
}

func (render *JSONRender) toJSON(val any) (string, error) {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return "", err
	}

	if len(jsonBytes) == 0 {
		return "", nil
	}

	jsonBytes = append(jsonBytes, '\n')

	return string(jsonBytes), nil
}
