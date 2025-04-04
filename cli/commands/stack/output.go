package stack

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/hashicorp/hcl/v2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func PrintRawOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	valueMap := outputs.AsValueMap()

	if len(valueMap) > 1 {
		// return error since in raw mode we want to print only one output
		return errors.New("multiple outputs found, please specify only one index")
	}

	for key, value := range valueMap {
		valueStr, err := getValueString(value)
		if err != nil {
			opts.Logger.Warnf("Error fetching output for '%s': %v", key, err)
			continue
		}

		line := valueStr + "\n"
		if _, err := writer.Write([]byte(line)); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

func getValueString(value cty.Value) (string, error) {
	if value.Type() == cty.String {
		return value.AsString(), nil
	}

	return config.CtyValueAsString(value)
}

func PrintOutputs(writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	for key, val := range outputs.AsValueMap() {
		rootBody.SetAttributeRaw(key, hclwrite.TokensForValue(val))
	}

	if _, err := writer.Write(f.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func PrintJSONOutput(writer io.Writer, outputs cty.Value) error {
	if outputs == cty.NilVal {
		return nil
	}

	rawJSON, err := ctyjson.Marshal(outputs, outputs.Type())

	if err != nil {
		return errors.New(err)
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, rawJSON, "", "  "); err != nil {
		return errors.New(err)
	}

	if _, err := writer.Write(pretty.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}
