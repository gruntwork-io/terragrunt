package view

import (
	"encoding/json"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
)

type JSONRender struct{}

func NewJSONRender() Render {
	return &JSONRender{}
}

func (render *JSONRender) Diagnostics(diags diagnostic.Diagnostics) (string, error) {
	return render.toJSON(diags)
}

func (render *JSONRender) ShowConfigPath(filenames []string) (string, error) {
	return render.toJSON(filenames)
}

func (render *JSONRender) toJSON(val any) (string, error) {
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return "", errors.New(err)
	}

	if len(jsonBytes) == 0 {
		return "", nil
	}

	jsonBytes = append(jsonBytes, '\n')

	return string(jsonBytes), nil
}
