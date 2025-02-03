package stack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

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

func printOutputs(opts *options.TerragruntOptions, writer io.Writer, outputs map[string]map[string]cty.Value, raw bool) error {
	for unit, values := range outputs {
		for key, value := range values {
			valueStr, err := getValueString(value, raw)
			if err != nil {
				opts.Logger.Warnf("Error fetching output from unit %s with key: %s", unit, key)
				continue
			}
			line := fmt.Sprintf("%s.%s = %s\n", unit, key, valueStr)
			if _, err := writer.Write([]byte(line)); err != nil {
				return errors.New(err)
			}
		}
	}
	return nil
}

func printJsonOutput(opts *options.TerragruntOptions, writer io.Writer, outputs map[string]map[string]cty.Value) error {
	outer := make(map[string]cty.Value)
	for unit, values := range outputs {
		outer[unit] = cty.ObjectVal(values)
	}

	topVal := cty.ObjectVal(outer)

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

func getValueString(value cty.Value, raw bool) (string, error) {
	if raw && value.Type() == cty.String {
		return value.AsString(), nil
	}
	return config.CtyValueAsString(value)
}
