package run

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// TFVarsFile is the auto-loaded file Terragrunt writes the inputs of tfvar variables blocks to.
	TFVarsFile = "terragrunt.auto.tfvars"
	// JSONVarsFile is the auto-loaded file Terragrunt writes the inputs of json variables blocks to.
	JSONVarsFile = "terragrunt.auto.tfvars.json"
)

// writeVarFiles writes the tfvar and json inputs of cfg to [TFVarsFile] and [JSONVarsFile] in opts.CacheDir.
//
// In a cache directory, the files stay after the run so OpenTofu/Terraform can be run there directly, and the file
// of a kind with no inputs is removed so a stale one is not loaded. In the unit directory, the run stops with
// [VarsFileExists] rather than replace a file, and the returned paths are the files written, which the caller removes
// after the run.
func writeVarFiles(fsys vfs.FS, opts *Options, cfg *runcfg.RunConfig) ([]string, error) {
	dir := varsFileDirOf(opts)

	var written []string

	for _, file := range []struct {
		inputs map[string]any
		encode func(map[string]any) ([]byte, error)
		name   string
	}{
		{inputs: cfg.TFVarsInputs, encode: encodeTFVars, name: TFVarsFile},
		{inputs: cfg.JSONInputs, encode: encodeJSONVars, name: JSONVarsFile},
	} {
		path := filepath.Join(opts.CacheDir, file.name)

		if len(file.inputs) == 0 {
			if err := removeStaleVarsFile(fsys, opts, path, dir); err != nil {
				return written, err
			}

			continue
		}

		contents, err := file.encode(file.inputs)
		if err != nil {
			return written, err
		}

		if err := writeVarsFile(fsys, path, contents, dir); err != nil {
			return written, err
		}

		if dir == varsFileDirUnit {
			written = append(written, path)
		}
	}

	return written, nil
}

// removeStaleVarsFile removes any file at path in a cache directory, so OpenTofu/Terraform does not load a file an
// earlier run left there. With the variables-block experiment on, [TFVarsFile] and [JSONVarsFile] are reserved names in
// a cache directory, so this also removes a file with that name copied from the unit or written by a generate block.
// With it off, no run wrote the file, so it is left alone. A file in the unit directory is left alone too, since a run
// there removes its own files and anything left belongs to the user.
func removeStaleVarsFile(fsys vfs.FS, opts *Options, path string, dir varsFileDir) error {
	if dir == varsFileDirUnit || !opts.Experiments.Evaluate(experiment.VariablesBlock) {
		return nil
	}

	if err := fsys.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// encodeTFVars renders inputs as a .tfvars file, sorted by name.
//
// Returns [InvalidVariableNameError] when a name is not an HCL identifier, which a .tfvars file cannot assign.
func encodeTFVars(inputs map[string]any) ([]byte, error) {
	asJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}

	var values ctyjson.SimpleJSONValue
	if err := values.UnmarshalJSON(asJSON); err != nil {
		return nil, err
	}

	file := hclwrite.NewFile()
	body := file.Body()

	for _, name := range slices.Sorted(maps.Keys(inputs)) {
		if !hclsyntax.ValidIdentifier(name) {
			return nil, InvalidVariableNameError{Name: name}
		}

		body.SetAttributeRaw(name, tfvarsTokens(values.GetAttr(name)))
	}

	return hclwrite.Format(file.Bytes()), nil
}

// tfvarsTokens renders value with every object key quoted. hclwrite leaves identifier keys bare, and a bare "for" as
// the first key of an object parses as a for expression.
func tfvarsTokens(value cty.Value) hclwrite.Tokens {
	ty := value.Type()

	if ty.IsTupleType() {
		elements := make([]hclwrite.Tokens, 0, value.LengthInt())

		for it := value.ElementIterator(); it.Next(); {
			_, element := it.Element()
			elements = append(elements, tfvarsTokens(element))
		}

		return hclwrite.TokensForTuple(elements)
	}

	if ty.IsObjectType() {
		attrs := make([]hclwrite.ObjectAttrTokens, 0, value.LengthInt())

		for it := value.ElementIterator(); it.Next(); {
			key, element := it.Element()
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForValue(key),
				Value: tfvarsTokens(element),
			})
		}

		return hclwrite.TokensForObject(attrs)
	}

	return hclwrite.TokensForValue(value)
}

// encodeJSONVars renders inputs as a .tfvars.json file. OpenTofu/Terraform read its strings literally, so a "${"
// sequence needs no escaping.
func encodeJSONVars(inputs map[string]any) ([]byte, error) {
	return json.MarshalIndent(inputs, "", "  ")
}

// validateDeclaredInputs checks that the module in modulePath declares every variable named in validated.
//
// Returns [UndeclaredInputsError] naming the variables the module does not declare.
func validateDeclaredInputs(fsys vfs.FS, modulePath string, validated map[string]struct{}) error {
	if len(validated) == 0 {
		return nil
	}

	declared, err := tf.ModuleVariables(fsys, modulePath)
	if err != nil {
		return err
	}

	var undeclared []string

	for name := range validated {
		if _, ok := declared[name]; !ok {
			undeclared = append(undeclared, name)
		}
	}

	if len(undeclared) == 0 {
		return nil
	}

	slices.Sort(undeclared)

	return UndeclaredInputsError{ModulePath: modulePath, Names: undeclared}
}
