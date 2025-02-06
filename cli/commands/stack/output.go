package stack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

func generateOutput(ctx context.Context, opts *options.TerragruntOptions) (map[string]map[string]cty.Value, error) {
	opts.TerragruntStackConfigPath = filepath.Join(opts.WorkingDir, defaultStackFile)
	opts.Logger.Debugf("Generating output from %s", opts.TerragruntStackConfigPath)
	stackFile, err := config.ReadStackConfigFile(ctx, opts)

	if err != nil {
		return nil, errors.New(err)
	}

	unitOutputs := make(map[string]map[string]cty.Value)
	// process each unit and get outputs
	for _, unit := range stackFile.Units {
		opts.Logger.Debugf("Processing unit %s", unit.Name)
		output, err := unit.ReadOutputs(ctx, opts)

		if err != nil {
			return nil, errors.New(err)
		}

		unitOutputs[unit.Name] = output
	}

	return unitOutputs, nil
}
func printRawOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	for key, value := range filteredOutputs {
		valueStr, err := getValueString(value)
		if err != nil {
			opts.Logger.Warnf("Error fetching output for '%s': %v", key, err)
			continue
		}

		line := fmt.Sprintf("%s = %s\n", key, valueStr)
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

func PrintOutputs(writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	rootBody := f.Body()

	for key, value := range filteredOutputs {
		tokens := hclwrite.TokensForValue(value)
		rootBody.SetAttributeRaw(key, tokens)
	}

	if _, err := writer.Write(f.Bytes()); err != nil {
		return errors.New(err)
	}

	return nil
}

func PrintJSONOutput(writer io.Writer, outputs map[string]map[string]cty.Value, outputIndex string) error {
	filteredOutputs := FilterOutputs(outputs, outputIndex)

	if filteredOutputs == nil {
		return nil
	}

	topVal := cty.ObjectVal(filteredOutputs)
	rawJSON, err := ctyjson.Marshal(topVal, topVal.Type())

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

func FilterOutputs(outputs map[string]map[string]cty.Value, outputIndex string) map[string]cty.Value {
	if outputIndex == "" {
		flattened := make(map[string]cty.Value)
		for unit, values := range outputs {
			flattened[unit] = cty.ObjectVal(values)
		}

		return flattened
	}

	keys := strings.Split(outputIndex, ".")
	currentMap := make(map[string]cty.Value)

	for unit, values := range outputs {
		if !strings.HasPrefix(outputIndex, unit) {
			continue
		}

		value := cty.ObjectVal(values)
		for _, key := range keys[1:] {
			if value.Type().IsObjectType() {
				mapVal := value.AsValueMap()
				if v, exists := mapVal[key]; exists {
					value = v
				} else {
					return nil
				}
			} else {
				return nil
			}
		}

		currentMap[outputIndex] = value
	}

	return currentMap
}
