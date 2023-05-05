package helpers

import (
	"bytes"
	"encoding/json"
)

type TerraformOutput struct {
	Sensitive bool
	Type      interface{}
	Value     interface{}
}

func ParseTerraformOutput(data []byte) (map[string]TerraformOutput, error) {

	outputs := map[string]TerraformOutput{}

	if index := bytes.IndexByte(data, byte('{')); index > 0 {
		data = data[index:]
	}

	err := json.Unmarshal(data, &outputs)

	return outputs, err
}
